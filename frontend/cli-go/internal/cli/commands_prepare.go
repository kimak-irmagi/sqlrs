package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/daemon"
	"github.com/sqlrs/cli/internal/remotesource"
	"github.com/sqlrs/cli/internal/util"

	"golang.org/x/term"
)

var (
	isTerminal                 = term.IsTerminal
	getTermSize                = term.GetSize
	makeRawTerminal            = term.MakeRaw
	restoreTerminalState       = term.Restore
	canUsePrepareControlPrompt = defaultCanUsePrepareControlPrompt
	promptPrepareControl       = promptPrepareControlDefault
)

type PrepareDetachedError struct {
	JobID string
}

func (e *PrepareDetachedError) Error() string {
	if strings.TrimSpace(e.JobID) == "" {
		return "prepare detached"
	}
	return fmt.Sprintf("prepare detached from job %s", e.JobID)
}

type prepareControlAction int

const (
	prepareControlContinue prepareControlAction = iota
	prepareControlDetach
	prepareControlStop
)

type waitPrepareOptions struct {
	allowControls bool
}

type PrepareOptions struct {
	ProfileName     string
	Mode            string
	AuthToken       string
	Endpoint        string
	Autostart       bool
	DaemonPath      string
	RunDir          string
	StateDir        string
	EngineRunDir    string
	EngineStatePath string
	EngineStoreDir  string
	WSLVHDXPath     string
	WSLMountUnit    string
	WSLMountFSType  string
	WSLDistro       string
	Timeout         time.Duration
	IdleTimeout     time.Duration
	StartupTimeout  time.Duration
	Verbose         bool

	ImageID          string
	PsqlArgs         []string
	LiquibaseArgs    []string
	LiquibaseVersion string
	WorkDir          string
	Stdin            *string
	PrepareKind      string
	PlanOnly         bool
	CompositeRun     bool
	// DisableControlPrompt prevents interactive detach/stop controls when the
	// caller cannot safely release temporary prepare inputs before job completion.
	DisableControlPrompt bool
	SourceSyncMode       string
	SourceSyncMaxRounds  int
	SourceSync           *remotesource.Options
}

func RunPrepare(ctx context.Context, opts PrepareOptions) (client.PrepareJobResult, error) {
	if opts.PlanOnly {
		return client.PrepareJobResult{}, fmt.Errorf("plan-only is not supported by RunPrepare")
	}
	cliClient, accepted, err := createPrepareJob(ctx, opts, false)
	if err != nil {
		return client.PrepareJobResult{}, err
	}

	jobID := accepted.JobID
	if jobID == "" {
		return client.PrepareJobResult{}, fmt.Errorf("prepare job id missing")
	}

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "waiting for prepare job %s\n", jobID)
	}

	eventsURL := strings.TrimSpace(accepted.EventsURL)
	if eventsURL == "" {
		return client.PrepareJobResult{}, fmt.Errorf("prepare events url missing")
	}

	status, err := waitForPrepareWithOptions(ctx, cliClient, jobID, eventsURL, os.Stderr, opts.Verbose, waitPrepareOptions{
		allowControls: !opts.DisableControlPrompt,
	})
	if err != nil {
		return client.PrepareJobResult{}, err
	}
	if status.Result == nil {
		return client.PrepareJobResult{}, fmt.Errorf("prepare job succeeded without result")
	}
	return *status.Result, nil
}

type PlanResult struct {
	PrepareKind           string            `json:"prepare_kind"`
	ImageID               string            `json:"image_id"`
	PrepareArgsNormalized string            `json:"prepare_args_normalized"`
	Tasks                 []client.PlanTask `json:"tasks"`
}

func RunPlan(ctx context.Context, opts PrepareOptions) (PlanResult, error) {
	opts.PlanOnly = true
	cliClient, accepted, err := createPrepareJob(ctx, opts, true)
	if err != nil {
		return PlanResult{}, err
	}

	jobID := accepted.JobID
	if jobID == "" {
		return PlanResult{}, fmt.Errorf("prepare job id missing")
	}

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "waiting for prepare job %s\n", jobID)
	}

	eventsURL := strings.TrimSpace(accepted.EventsURL)
	if eventsURL == "" {
		return PlanResult{}, fmt.Errorf("prepare events url missing")
	}

	status, err := waitForPrepareWithOptions(ctx, cliClient, jobID, eventsURL, os.Stderr, opts.Verbose, waitPrepareOptions{})
	if err != nil {
		return PlanResult{}, err
	}
	if !status.PlanOnly {
		return PlanResult{}, fmt.Errorf("prepare job is not plan-only")
	}
	if len(status.Tasks) == 0 {
		return PlanResult{}, fmt.Errorf("plan job succeeded without tasks")
	}
	return PlanResult{
		PrepareKind:           status.PrepareKind,
		ImageID:               status.ImageID,
		PrepareArgsNormalized: status.PrepareArgsNormalized,
		Tasks:                 status.Tasks,
	}, nil
}

func SubmitPrepare(ctx context.Context, opts PrepareOptions) (client.PrepareJobAccepted, error) {
	_, accepted, err := createPrepareJob(ctx, opts, false)
	if err != nil {
		return client.PrepareJobAccepted{}, err
	}
	return accepted, nil
}

func RunWatch(ctx context.Context, opts PrepareOptions, jobID string) (client.PrepareJobStatus, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return client.PrepareJobStatus{}, fmt.Errorf("prepare job id is required")
	}
	requestedJobID := jobID
	cliClient, err := prepareClient(ctx, opts)
	if err != nil {
		return client.PrepareJobStatus{}, err
	}
	status, found, err := cliClient.GetPrepareJob(ctx, jobID)
	if err != nil {
		return client.PrepareJobStatus{}, err
	}
	if !found {
		resolvedJobID, err := resolvePrepareJobByPrefix(ctx, cliClient, requestedJobID)
		if err != nil {
			return client.PrepareJobStatus{}, err
		}
		if resolvedJobID != "" {
			jobID = resolvedJobID
			status, found, err = cliClient.GetPrepareJob(ctx, jobID)
			if err != nil {
				return client.PrepareJobStatus{}, err
			}
		}
		if !found {
			return client.PrepareJobStatus{}, fmt.Errorf("prepare job not found: %s", requestedJobID)
		}
	}
	switch status.Status {
	case "succeeded":
		return status, nil
	case "failed":
		return client.PrepareJobStatus{}, prepareFailureError(status, nil)
	case "recovery_required":
		return client.PrepareJobStatus{}, prepareRecoveryError(status)
	}
	eventsURL := "/v1/prepare-jobs/" + jobID + "/events"
	return waitForPrepareWithOptions(ctx, cliClient, jobID, eventsURL, os.Stderr, opts.Verbose, waitPrepareOptions{
		allowControls: !opts.DisableControlPrompt,
	})
}

func resolvePrepareJobByPrefix(ctx context.Context, cliClient *client.Client, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", nil
	}
	jobs, err := cliClient.ListPrepareJobs(ctx, prefix)
	if err != nil {
		var statusErr *client.HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return "", nil
		}
		return "", err
	}
	if len(jobs) == 0 {
		return "", nil
	}
	if len(jobs) > 1 {
		return "", &AmbiguousPrefixError{Kind: "job", Prefix: prefix}
	}
	return jobs[0].JobID, nil
}

func prepareClient(ctx context.Context, opts PrepareOptions) (*client.Client, error) {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	endpoint := strings.TrimSpace(opts.Endpoint)
	authToken := strings.TrimSpace(opts.AuthToken)

	if mode == "local" {
		authToken = ""
		if endpoint == "" {
			endpoint = "auto"
		}
		if endpoint == "auto" {
			if opts.Verbose {
				fmt.Fprintln(os.Stderr, "checking local engine state")
			}
			resolved, err := daemon.ConnectOrStart(ctx, daemon.ConnectOptions{
				Endpoint:        endpoint,
				Autostart:       opts.Autostart,
				DaemonPath:      opts.DaemonPath,
				RunDir:          opts.RunDir,
				StateDir:        opts.StateDir,
				EngineRunDir:    opts.EngineRunDir,
				EngineStatePath: opts.EngineStatePath,
				EngineStoreDir:  opts.EngineStoreDir,
				WSLVHDXPath:     opts.WSLVHDXPath,
				WSLMountUnit:    opts.WSLMountUnit,
				WSLMountFSType:  opts.WSLMountFSType,
				WSLDistro:       opts.WSLDistro,
				IdleTimeout:     opts.IdleTimeout,
				StartupTimeout:  opts.StartupTimeout,
				ClientTimeout:   opts.Timeout,
				Verbose:         opts.Verbose,
			})
			if err != nil {
				return nil, err
			}
			endpoint = resolved.Endpoint
			authToken = resolved.AuthToken
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "engine ready at %s\n", endpoint)
			}
		}
	} else if mode == "remote" {
		if endpoint == "" || endpoint == "auto" {
			return nil, fmt.Errorf("remote mode requires explicit endpoint")
		}
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "using remote endpoint %s\n", endpoint)
		}
	}

	cliClient := client.New(endpoint, client.Options{Timeout: opts.Timeout, AuthToken: authToken})
	return cliClient, nil
}

func PrepareClient(ctx context.Context, opts PrepareOptions) (*client.Client, error) {
	return prepareClient(ctx, opts)
}

func waitForPrepare(ctx context.Context, cliClient *client.Client, jobID string, eventsURL string, progress io.Writer, verbose bool) (client.PrepareJobStatus, error) {
	return waitForPrepareWithOptions(ctx, cliClient, jobID, eventsURL, progress, verbose, waitPrepareOptions{})
}

func createPrepareJob(ctx context.Context, opts PrepareOptions, planOnly bool) (*client.Client, client.PrepareJobAccepted, error) {
	cliClient, err := prepareClient(ctx, opts)
	if err != nil {
		return nil, client.PrepareJobAccepted{}, err
	}

	if opts.Verbose {
		if planOnly {
			fmt.Fprintln(os.Stderr, "submitting prepare job (plan-only)")
		} else {
			fmt.Fprintln(os.Stderr, "submitting prepare job")
		}
	}
	request := BuildPrepareJobRequest(opts, planOnly)
	accepted, err := createPrepareJobWithSourceSync(ctx, cliClient, opts, request)
	if err != nil {
		return nil, client.PrepareJobAccepted{}, err
	}
	return cliClient, accepted, nil
}

// BuildPrepareJobRequest implements the container-only request boundary in
// docs/architecture/liquibase-container-component-structure.md. No profile
// sends client-side Liquibase runner settings to the engine.
func BuildPrepareJobRequest(opts PrepareOptions, planOnly bool) client.PrepareJobRequest {
	prepareKind := strings.TrimSpace(opts.PrepareKind)
	if prepareKind == "" {
		prepareKind = "psql"
	}
	request := client.PrepareJobRequest{
		PrepareKind:      prepareKind,
		ImageID:          opts.ImageID,
		PsqlArgs:         opts.PsqlArgs,
		LiquibaseArgs:    opts.LiquibaseArgs,
		LiquibaseVersion: opts.LiquibaseVersion,
		WorkDir:          opts.WorkDir,
		Stdin:            opts.Stdin,
		PlanOnly:         planOnly,
	}
	return request
}

func createPrepareJobWithSourceSync(ctx context.Context, cliClient *client.Client, opts PrepareOptions, request client.PrepareJobRequest) (client.PrepareJobAccepted, error) {
	if opts.SourceSync == nil || !opts.SourceSync.Enabled {
		return cliClient.CreatePrepareJob(ctx, request)
	}
	syncOpts := *opts.SourceSync
	syncOpts.Uploader = cliClient
	return remotesource.Execute(ctx, syncOpts, request, func(ctx context.Context, req client.PrepareJobRequest) (client.PrepareJobAccepted, error) {
		return cliClient.CreatePrepareJob(ctx, req)
	})
}

func waitForPrepareWithOptions(ctx context.Context, cliClient *client.Client, jobID string, eventsURL string, progress io.Writer, verbose bool, options waitPrepareOptions) (client.PrepareJobStatus, error) {
	if strings.TrimSpace(eventsURL) == "" {
		return client.PrepareJobStatus{}, fmt.Errorf("prepare events url missing")
	}
	tracker := newPrepareProgress(progress, verbose)
	defer tracker.Close()

	controlsEnabled := options.allowControls && canUsePrepareControlPrompt(progress)
	interrupts := make(chan os.Signal, 2)
	if controlsEnabled {
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
	}

	resumeIndex := 0
	for {
		rangeHeader := ""
		if resumeIndex > 0 {
			rangeHeader = fmt.Sprintf("events=%d-", resumeIndex)
		}
		streamCtx := ctx
		streamCancel := func() {}
		var interruptFired int32
		stopInterruptWatch := make(chan struct{})
		cleanupInterrupt := func() {
			if controlsEnabled {
				select {
				case <-stopInterruptWatch:
				default:
					close(stopInterruptWatch)
				}
			}
			streamCancel()
		}
		if controlsEnabled {
			streamCtx, streamCancel = context.WithCancel(ctx)
			go func() {
				select {
				case <-stopInterruptWatch:
				case <-interrupts:
					atomic.StoreInt32(&interruptFired, 1)
					streamCancel()
				}
			}()
		}

		resp, err := cliClient.StreamPrepareEvents(streamCtx, eventsURL, rangeHeader)
		if err != nil {
			cleanupInterrupt()
			if controlsEnabled && atomic.LoadInt32(&interruptFired) == 1 && ctx.Err() == nil {
				status, actionErr := handlePrepareControlAction(ctx, cliClient, jobID, tracker, interrupts)
				if actionErr != nil {
					return client.PrepareJobStatus{}, actionErr
				}
				if status != nil {
					return *status, nil
				}
				continue
			}
			if ctx.Err() != nil {
				return client.PrepareJobStatus{}, ctx.Err()
			}
			return client.PrepareJobStatus{}, err
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			resp.Body.Close()
			cleanupInterrupt()
			return client.PrepareJobStatus{}, fmt.Errorf("events stream returned status %d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			cleanupInterrupt()
			return client.PrepareJobStatus{}, fmt.Errorf("events stream returned status %d", resp.StatusCode)
		}

		startIndex := 0
		if resp.StatusCode == http.StatusPartialContent {
			rangeStart, err := parseEventsContentRange(resp.Header.Get("Content-Range"))
			if err != nil {
				resp.Body.Close()
				cleanupInterrupt()
				return client.PrepareJobStatus{}, err
			}
			startIndex = rangeStart
			if rangeStart > resumeIndex {
				resumeIndex = rangeStart
			}
		}

		reader := util.NewNDJSONReader(resp.Body)
		currentIndex := startIndex
		readErr := error(nil)
		for {
			line, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				readErr = err
				break
			}
			if len(line) == 0 {
				continue
			}
			if currentIndex < resumeIndex {
				currentIndex++
				continue
			}
			var event client.PrepareJobEvent
			if err := json.Unmarshal(line, &event); err != nil {
				resp.Body.Close()
				cleanupInterrupt()
				return client.PrepareJobStatus{}, err
			}
			tracker.Update(event)
			// A03c in docs/architecture/liquibase-container-tests.md:
			// a terminal job-state event can be an error without a separate
			// status event. The saved job status remains authoritative.
			if event.Type == "status" || event.Status == "failed" || event.Status == "succeeded" || event.Status == "recovery_required" {
				status, found, err := cliClient.GetPrepareJob(ctx, jobID)
				if err != nil {
					resp.Body.Close()
					cleanupInterrupt()
					return client.PrepareJobStatus{}, err
				}
				if !found {
					resp.Body.Close()
					cleanupInterrupt()
					return client.PrepareJobStatus{}, fmt.Errorf("prepare job not found: %s", jobID)
				}
				switch status.Status {
				case "succeeded":
					resp.Body.Close()
					cleanupInterrupt()
					return status, nil
				case "failed":
					resp.Body.Close()
					cleanupInterrupt()
					return client.PrepareJobStatus{}, prepareFailureError(status, tracker)
				case "recovery_required":
					resp.Body.Close()
					cleanupInterrupt()
					return client.PrepareJobStatus{}, prepareRecoveryError(status)
				}
			}
			currentIndex++
			resumeIndex = currentIndex
		}
		resp.Body.Close()
		cleanupInterrupt()
		if controlsEnabled && atomic.LoadInt32(&interruptFired) == 1 && ctx.Err() == nil {
			status, actionErr := handlePrepareControlAction(ctx, cliClient, jobID, tracker, interrupts)
			if actionErr != nil {
				return client.PrepareJobStatus{}, actionErr
			}
			if status != nil {
				return *status, nil
			}
			continue
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return client.PrepareJobStatus{}, ctx.Err()
			}
			continue
		}
		// A complete 200 response is the end of the stream regardless of
		// whether HTTP declared its length. Only an interrupted read or a
		// partial-content response may reconnect. See the completion rule in
		// docs/architecture/prepare-job-events-flow.md.
		if resp.StatusCode == http.StatusOK {
			return client.PrepareJobStatus{}, fmt.Errorf("prepare job events stream ended without terminal status")
		}
		if ctx.Err() != nil {
			return client.PrepareJobStatus{}, ctx.Err()
		}
	}
}

func handlePrepareControlAction(ctx context.Context, cliClient *client.Client, jobID string, tracker *prepareProgress, interrupts <-chan os.Signal) (*client.PrepareJobStatus, error) {
	status, found, err := cliClient.GetPrepareJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("prepare job not found: %s", jobID)
	}
	switch status.Status {
	case "succeeded":
		return &status, nil
	case "failed":
		return nil, prepareFailureError(status, tracker)
	case "recovery_required":
		return nil, prepareRecoveryError(status)
	}

	drainInterruptSignals(interrupts)
	action, err := promptPrepareControl(tracker.writer, interrupts)
	if err != nil {
		return nil, err
	}
	switch action {
	case prepareControlDetach:
		return nil, &PrepareDetachedError{JobID: jobID}
	case prepareControlStop:
		cancelStatus, _, err := cliClient.CancelPrepareJob(ctx, jobID)
		if err != nil {
			return nil, err
		}
		switch cancelStatus.Status {
		case "succeeded":
			return &cancelStatus, nil
		case "failed":
			return nil, prepareFailureError(cancelStatus, tracker)
		case "recovery_required":
			return nil, prepareRecoveryError(cancelStatus)
		default:
			return nil, nil
		}
	default:
		return nil, nil
	}
}

func drainInterruptSignals(interrupts <-chan os.Signal) {
	for {
		select {
		case <-interrupts:
			continue
		default:
			return
		}
	}
}

func prepareFailureError(status client.PrepareJobStatus, tracker *prepareProgress) error {
	if status.Error != nil {
		if tracker != nil {
			tracker.Update(client.PrepareJobEvent{
				Type:  "error",
				Error: status.Error,
			})
		}
		if status.Error.Details != "" {
			return fmt.Errorf("%s: %s", status.Error.Message, status.Error.Details)
		}
		return fmt.Errorf("%s", status.Error.Message)
	}
	return fmt.Errorf("prepare job failed")
}

// prepareRecoveryError reports a nonterminal safety stop without treating it
// as an ordinary failed job. See transformation-attempt-recovery-flow.md.
func prepareRecoveryError(status client.PrepareJobStatus) error {
	jobID := strings.TrimSpace(status.JobID)
	if jobID == "" {
		jobID = "unknown"
	}
	if status.Error == nil {
		return fmt.Errorf("prepare job %s requires recovery", jobID)
	}
	reason := strings.TrimSpace(status.Error.Details)
	if reason == "" {
		reason = strings.TrimSpace(status.Error.Message)
	}
	if reason == "" {
		return fmt.Errorf("prepare job %s requires recovery", jobID)
	}
	return fmt.Errorf("prepare job %s requires recovery: %s", jobID, reason)
}

func defaultCanUsePrepareControlPrompt(progress io.Writer) bool {
	progressFile, ok := progress.(*os.File)
	if !ok {
		return false
	}
	if !isTerminal(int(progressFile.Fd())) {
		return false
	}
	return isTerminal(int(os.Stdin.Fd()))
}

func promptPrepareControlDefault(writer io.Writer, interrupts <-chan os.Signal) (prepareControlAction, error) {
	if writer == nil {
		writer = io.Discard
	}
	restoreRaw := enableRawStdin()
	defer restoreRaw()
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "[s] stop  [d] detach  [Esc/Enter] continue")
	fmt.Fprint(writer, "> ")
	for {
		select {
		case <-interrupts:
			fmt.Fprintln(writer)
			return prepareControlContinue, nil
		default:
		}
		b, err := reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return prepareControlContinue, nil
			}
			return prepareControlContinue, err
		}
		switch b {
		case 3:
			// Ctrl+C in raw mode
			fmt.Fprintln(writer)
			return prepareControlContinue, nil
		case '\r', '\n', 27:
			return prepareControlContinue, nil
		case 'd', 'D':
			return prepareControlDetach, nil
		case 's', 'S':
			consumeBufferedLineEnd(reader)
			confirmed, err := confirmPrepareStop(reader, writer, interrupts)
			if err != nil {
				return prepareControlContinue, err
			}
			if confirmed {
				return prepareControlStop, nil
			}
			return prepareControlContinue, nil
		default:
		}
	}
}

func enableRawStdin() func() {
	fd := int(os.Stdin.Fd())
	if !isTerminal(fd) {
		return func() {}
	}
	state, err := makeRawTerminal(fd)
	if err != nil {
		return func() {}
	}
	return func() {
		_ = restoreTerminalState(fd, state)
	}
}

func consumeBufferedLineEnd(reader *bufio.Reader) {
	for reader.Buffered() > 0 {
		next, err := reader.Peek(1)
		if err != nil || len(next) == 0 {
			return
		}
		if next[0] != '\r' && next[0] != '\n' {
			return
		}
		_, _ = reader.ReadByte()
	}
}

func confirmPrepareStop(reader *bufio.Reader, writer io.Writer, interrupts <-chan os.Signal) (bool, error) {
	fmt.Fprint(writer, "Cancel job? [y/N] ")
	for {
		select {
		case <-interrupts:
			fmt.Fprintln(writer)
			return false, nil
		default:
		}
		b, err := reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			return false, err
		}
		switch b {
		case 3:
			// Ctrl+C in raw mode
			fmt.Fprintln(writer)
			return false, nil
		case 'y', 'Y':
			fmt.Fprintln(writer)
			return true, nil
		case '\r', '\n', 27, 'n', 'N':
			fmt.Fprintln(writer)
			return false, nil
		default:
		}
	}
}

func parseEventsContentRange(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("missing content range")
	}
	if !strings.HasPrefix(value, "events ") {
		return 0, fmt.Errorf("unexpected content range: %s", value)
	}
	value = strings.TrimPrefix(value, "events ")
	parts := strings.SplitN(value, "/", 2)
	rangePart := parts[0]
	dash := strings.Index(rangePart, "-")
	if dash == -1 {
		return 0, fmt.Errorf("invalid content range: %s", value)
	}
	start, err := strconv.Atoi(strings.TrimSpace(rangePart[:dash]))
	if err != nil {
		return 0, fmt.Errorf("invalid content range: %s", value)
	}
	return start, nil
}

type prepareProgress struct {
	writer       io.Writer
	spinner      []string
	spinnerIndex int
	lastKey      string
	lastLen      int
	lastVisible  int
	wroteLine    bool
	verbose      bool
	clearLine    bool
	minSpinner   bool
	spinnerShown bool
}

func newPrepareProgress(writer io.Writer, verbose bool) *prepareProgress {
	if writer == nil {
		writer = io.Discard
	}
	return &prepareProgress{
		writer:     writer,
		spinner:    []string{"-", "\\", "|", "/"},
		verbose:    verbose,
		clearLine:  supportsAnsiClear(writer),
		minSpinner: supportsSpinnerBackspace(writer),
	}
}

func (p *prepareProgress) Update(event client.PrepareJobEvent) {
	base := formatPrepareEvent(event)
	if p.verbose {
		if p.writer != io.Discard {
			fmt.Fprintln(p.writer, base)
			p.wroteLine = true
		}
		return
	}
	key := prepareEventKey(event)
	if key == p.lastKey && p.wroteLine {
		p.spinnerIndex = (p.spinnerIndex + 1) % len(p.spinner)
		if p.minSpinner {
			p.writeSpinner(p.spinner[p.spinnerIndex])
			return
		}
		p.writeLine(base + " " + p.spinner[p.spinnerIndex])
		return
	}
	p.lastKey = key
	p.spinnerIndex = 0
	p.spinnerShown = false
	p.writeLine(base)
}

func (p *prepareProgress) Close() {
	if p.writer == io.Discard || !p.wroteLine || p.verbose {
		return
	}
	fmt.Fprint(p.writer, "\n")
}

func (p *prepareProgress) writeLine(line string) {
	if p.writer == io.Discard {
		return
	}
	width, hasWidth := terminalWidth(p.writer)
	if hasWidth && width > 0 {
		line = truncateLine(line, width)
	}
	if p.wroteLine {
		if p.clearLine {
			fmt.Fprint(p.writer, "\r\033[2K")
		} else {
			fmt.Fprint(p.writer, "\r")
		}
	}
	fmt.Fprint(p.writer, line)
	visible := len([]rune(line))
	if !p.clearLine && p.wroteLine && p.lastVisible > visible {
		padding := p.lastVisible - visible
		if hasWidth && width > 0 && visible+padding >= width {
			padding = max(0, width-1-visible)
		}
		if padding > 0 {
			fmt.Fprint(p.writer, strings.Repeat(" ", padding))
		}
	}
	p.lastLen = len(line)
	p.lastVisible = visible
	p.wroteLine = true
}

func supportsAnsiClear(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	if !isTerminal(int(file.Fd())) {
		return false
	}
	return runtime.GOOS != "windows" || os.Getenv("WT_SESSION") != "" || os.Getenv("TERM") != ""
}

func supportsSpinnerBackspace(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	if !isTerminal(int(file.Fd())) {
		return false
	}
	return runtime.GOOS != "windows" || os.Getenv("WT_SESSION") != "" || os.Getenv("TERM") != ""
}

func terminalWidth(writer io.Writer) (int, bool) {
	file, ok := writer.(*os.File)
	if !ok {
		return 0, false
	}
	if !isTerminal(int(file.Fd())) {
		return 0, false
	}
	width, _, err := getTermSize(int(file.Fd()))
	if err != nil || width <= 0 {
		return 0, false
	}
	return width, true
}

func truncateLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func (p *prepareProgress) writeSpinner(symbol string) {
	if p.writer == io.Discard {
		return
	}
	if width, ok := terminalWidth(p.writer); ok && width > 0 && p.lastVisible+2 > width {
		return
	}
	if p.spinnerShown {
		fmt.Fprint(p.writer, "\b"+symbol)
		return
	}
	fmt.Fprint(p.writer, " "+symbol)
	p.spinnerShown = true
}

func prepareEventKey(event client.PrepareJobEvent) string {
	return fmt.Sprintf("%s|%s|%s|%s", event.Type, event.Status, event.TaskID, event.Message)
}

func formatPrepareEvent(event client.PrepareJobEvent) string {
	message := strings.TrimSpace(event.Message)
	if message == "" && event.Error != nil {
		errMsg := strings.TrimSpace(event.Error.Message)
		errDetails := strings.TrimSpace(event.Error.Details)
		switch {
		case errMsg != "" && errDetails != "":
			message = errMsg + ": " + errDetails
		case errMsg != "":
			message = errMsg
		case errDetails != "":
			message = errDetails
		}
	}
	suffix := ""
	if message != "" {
		suffix = " - " + message
	}
	switch event.Type {
	case "status":
		if event.Status != "" {
			return fmt.Sprintf("prepare status: %s%s", event.Status, suffix)
		}
		return "prepare status" + suffix
	case "task":
		if event.TaskID != "" && event.Status != "" {
			return fmt.Sprintf("prepare task %s: %s%s", event.TaskID, event.Status, suffix)
		}
		if event.TaskID != "" {
			return fmt.Sprintf("prepare task %s%s", event.TaskID, suffix)
		}
		if event.Status != "" {
			return fmt.Sprintf("prepare task: %s%s", event.Status, suffix)
		}
		return "prepare task" + suffix
	case "result":
		return "prepare result: ready"
	case "error":
		if event.Error != nil {
			if event.Error.Details != "" {
				return fmt.Sprintf("prepare error: %s: %s", event.Error.Message, event.Error.Details)
			}
			if event.Error.Message != "" {
				return fmt.Sprintf("prepare error: %s", event.Error.Message)
			}
		}
		return "prepare error"
	default:
		if message != "" {
			return fmt.Sprintf("prepare %s: %s", event.Type, message)
		}
		return fmt.Sprintf("prepare %s", event.Type)
	}
}
