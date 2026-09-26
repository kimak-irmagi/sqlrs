package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
)

const defaultTimeout = 30 * time.Second
const defaultStartupTimeout = 5 * time.Second
const defaultIdleTimeout = 120 * time.Second

var spinnerInitialDelay = 500 * time.Millisecond
var spinnerTickInterval = 150 * time.Millisecond

func writeJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func formatCleanupResult(result client.DeleteResult) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(result.Outcome) != "" {
		parts = append(parts, "outcome="+result.Outcome)
	}
	if strings.TrimSpace(result.Root.Blocked) != "" {
		parts = append(parts, "blocked="+result.Root.Blocked)
	}
	if result.Root.Connections != nil {
		parts = append(parts, fmt.Sprintf("connections=%d", *result.Root.Connections))
	}
	if len(parts) == 0 {
		return "blocked"
	}
	return strings.Join(parts, ", ")
}

func startCleanupSpinner(instanceID string, verbose bool) func() {
	label := fmt.Sprintf("Deleting instance %s", instanceID)
	out := os.Stdout
	if verbose || !isTerminalWriterFn(out) {
		fmt.Fprintln(out, label)
		return func() {}
	}

	clearLen := len(label) + 2
	done := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(finished)
		timer := time.NewTimer(spinnerInitialDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-done:
			return
		}
		spinner := []string{"-", "\\", "|", "/"}
		idx := 0
		ticker := time.NewTicker(spinnerTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				clearLineOut(out, clearLen)
				return
			case <-ticker.C:
				clearLineOut(out, clearLen)
				fmt.Fprintf(out, "%s %s", label, spinner[idx])
				idx = (idx + 1) % len(spinner)
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
}

func clearLineOut(out io.Writer, width int) {
	if out == nil {
		return
	}
	if width <= 0 {
		width = 1
	}
	fmt.Fprint(out, "\r")
	fmt.Fprint(out, strings.Repeat(" ", width))
	fmt.Fprint(out, "\r")
}

func resolveAuthToken(auth config.AuthConfig) string {
	env := strings.TrimSpace(auth.TokenEnv)
	if env == "" && (strings.EqualFold(strings.TrimSpace(auth.Mode), "oidcSession") || strings.EqualFold(strings.TrimSpace(auth.Mode), "remoteSession")) {
		env = "SQLRS_TOKEN"
	}
	if env != "" {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			return value
		}
	}
	if strings.EqualFold(strings.TrimSpace(auth.Mode), "oidcSession") || strings.EqualFold(strings.TrimSpace(auth.Mode), "remoteSession") {
		return ""
	}
	return strings.TrimSpace(auth.Token)
}
