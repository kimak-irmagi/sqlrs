package prepare

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

var execCommand = exec.CommandContext

type hostLiquibaseRunner struct{}

func (r hostLiquibaseRunner) Run(ctx context.Context, req LiquibaseRunRequest) (string, error) {
	if req.Credential != nil {
		if sink := engineRuntime.LogSinkFromContext(ctx); sink != nil {
			ctx = engineRuntime.WithLogSink(ctx, func(line string) { sink(engineRuntime.RedactManagedOutput(line, req.Credential.Password)) })
		}
	}
	execPath := strings.TrimSpace(req.ExecPath)
	if execPath == "" {
		execPath = "liquibase"
	}
	args := req.Args
	mode := normalizeExecMode(req.ExecMode)
	useWindows := shouldUseWindowsBat(execPath, mode)
	sink := engineRuntime.LogSinkFromContext(ctx)
	if sink != nil {
		sink(fmt.Sprintf("exec: raw exec_path=%q", execPath))
		if len(args) == 0 {
			sink("exec: raw args=<empty>")
		} else {
			sink(fmt.Sprintf("exec: raw args=%q", args))
		}
		sink(fmt.Sprintf("exec: mode=%s", mode))
	}
	if useWindows {
		workDir := strings.TrimSpace(req.WorkDir)
		if sink != nil {
			if workDir == "" {
				sink(fmt.Sprintf("exec: cmd.exe /c call %q ...", execPath))
			} else {
				sink(fmt.Sprintf("exec: cmd.exe /c cd /d %q && call %q ...", workDir, execPath))
			}
		}
		if workDir == "" {
			args = append([]string{"/c", "call", execPath}, args...)
		} else {
			args = append([]string{"/c", "cd", "/d", workDir, "&&", "call", execPath}, args...)
		}
		execPath = "cmd.exe"
	}
	cmd := execCommand(ctx, execPath, args...)
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), formatEnv(req.Env)...)
	}
	if req.Credential != nil {
		base := cmd.Env
		if base == nil {
			base = os.Environ()
		}
		cmd.Env = mergeEnv(base, map[string]string{"LIQUIBASE_COMMAND_PASSWORD": req.Credential.Password})
	}
	if !useWindows && strings.TrimSpace(req.WorkDir) != "" {
		cmd.Dir = req.WorkDir
	}
	output, err := runCommandWithSink(ctx, cmd)
	if req.Credential != nil {
		output = engineRuntime.RedactManagedOutput(output, req.Credential.Password)
		if err != nil {
			return output, fmt.Errorf("managed Liquibase execution failed")
		}
	}
	return output, err
}

func runCommandWithSink(ctx context.Context, cmd *exec.Cmd) (string, error) {
	sink := engineRuntime.LogSinkFromContext(ctx)
	if sink == nil {
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	pipeR, pipeW := io.Pipe()
	cmd.Stdout = pipeW
	cmd.Stderr = pipeW
	if err := cmd.Start(); err != nil {
		_ = pipeW.Close()
		return "", err
	}
	lineCh := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(pipeR)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lineCh <- strings.TrimRight(scanner.Text(), "\r")
		}
		close(lineCh)
	}()
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		_ = pipeW.Close()
	}()

	var output strings.Builder
	for line := range lineCh {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			sink(trimmed)
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	waitErr := <-waitCh
	return output.String(), waitErr
}

func formatEnv(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}

func normalizeExecMode(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "auto"
	}
	switch value {
	case "auto", "windows-bat", "native":
		return value
	default:
		return "auto"
	}
}

func shouldUseWindowsBat(execPath string, mode string) bool {
	switch mode {
	case "windows-bat":
		return true
	case "native":
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(execPath))
	return strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd")
}

func buildCmdLine(execPath string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteCmdArg(execPath))
	for _, arg := range args {
		parts = append(parts, quoteCmdArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteCmdArg(value string) string {
	if value == "" {
		return "\"\""
	}
	needsQuote := strings.ContainsAny(value, " \t\"")
	if !needsQuote {
		return value
	}
	escaped := strings.ReplaceAll(value, `"`, `""`)
	return `"` + escaped + `"`
}
