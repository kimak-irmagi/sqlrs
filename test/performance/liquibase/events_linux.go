package benchmark

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// benchmarkContainerCreate is the exact timestamp/image/name evidence used by
// the external collector; see issue89-performance-actions-tests.md PA06/HC01.
type benchmarkContainerCreate struct {
	at          int64
	image, name string
}

// benchmarkDockerCreates synchronizes stream ingestion with the test's barrier
// and counters. cmd/closed belong to the test orchestration; evidence and errors
// stay in ordinary, coverage-measured helpers (HC01–HC02).
type benchmarkDockerCreates struct {
	mu     sync.Mutex
	events []benchmarkContainerCreate
	err    error
	cmd    *exec.Cmd
	closed chan struct{}
}

// consumeEvents retains the existing bounded Docker scanner and exact validation.
// The caller owns the stream/process lifetime. Any invalid record or scanner
// failure invalidates interval counts, even if other records were complete.
func (r *benchmarkDockerCreates) consumeEvents(input io.Reader) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var event struct {
			Action   string
			TimeNano int64 `json:"timeNano"`
			Actor    struct{ Attributes map[string]string }
		}
		err := json.Unmarshal(scanner.Bytes(), &event)
		r.mu.Lock()
		if err != nil || event.Action != "create" || event.TimeNano <= 0 || event.Actor.Attributes["image"] == "" || event.Actor.Attributes["name"] == "" {
			r.err = fmt.Errorf("Docker create stream is incomplete or invalid")
		} else {
			r.events = append(r.events, benchmarkContainerCreate{at: event.TimeNano, image: event.Actor.Attributes["image"], name: event.Actor.Attributes["name"]})
		}
		r.mu.Unlock()
	}
	r.mu.Lock()
	if err := scanner.Err(); err != nil {
		r.err = err
	}
	r.mu.Unlock()
}

// count translates wall-clock interval endpoints without changing precision or
// inclusive bounds. The observer barrier remains outside the measured interval.
func (r *benchmarkDockerCreates) count(start, end time.Time, image string) (int, int, error) {
	return r.countNano(start.UnixNano(), end.UnixNano(), image)
}

// countNano reports all creates and separately pinned-image inspection creates.
// Never qualify a count from an incomplete stream (PA06/HC02).
func (r *benchmarkDockerCreates) countNano(start, end int64, image string) (int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return 0, 0, r.err
	}
	count := 0
	inspections := 0
	for _, event := range r.events {
		if event.at >= start && event.at <= end {
			count++
			// In both pinned container-engine sources, the only unnamed
			// Liquibase-image container is the offline version/JAR probe.
			// Migration executors have durable sqlrs-* names. A qualifying
			// pass rejects extra/missing probes instead of assuming one.
			if event.image == image && !strings.HasPrefix(event.name, "sqlrs-") {
				inspections++
			}
		}
	}
	return count, inspections, nil
}
