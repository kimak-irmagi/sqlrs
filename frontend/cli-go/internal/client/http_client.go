package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Options struct {
	Timeout               time.Duration
	SourceTransferTimeout time.Duration
	AuthToken             string
	UserAgent             string
}

type Client struct {
	baseURL    string
	http       *http.Client
	sourceHTTP *http.Client
	authToken  string
	userAgent  string
}

func New(baseURL string, opts Options) *Client {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	sourceTimeout := opts.SourceTransferTimeout
	if sourceTimeout == 0 {
		sourceTimeout = 15 * time.Minute
	}
	newHTTPClient := func(clientTimeout time.Duration) *http.Client {
		client := &http.Client{Timeout: clientTimeout}
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			prev := via[len(via)-1]
			if auth := prev.Header.Get("Authorization"); auth != "" {
				req.Header.Set("Authorization", auth)
			}
			if ua := prev.Header.Get("User-Agent"); ua != "" {
				req.Header.Set("User-Agent", ua)
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		return client
	}
	return &Client{
		baseURL:    normalizeBaseURL(baseURL),
		http:       newHTTPClient(timeout),
		sourceHTTP: newHTTPClient(sourceTimeout),
		authToken:  opts.AuthToken,
		userAgent:  opts.UserAgent,
	}
}

func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var out HealthResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/health", false, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) GetCacheStatus(ctx context.Context) (CacheStatus, error) {
	var out CacheStatus
	if err := c.doJSON(ctx, http.MethodGet, "/v1/cache/status", true, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) ExplainPrepareCache(ctx context.Context, req PrepareJobRequest) (CacheExplainPrepareResponse, error) {
	var out CacheExplainPrepareResponse
	body, err := json.Marshal(req)
	if err != nil {
		return out, err
	}
	resp, err := c.doRequestWithBody(ctx, http.MethodPost, "/v1/cache/explain/prepare", true, bytes.NewReader(body), "application/json")
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, parseErrorResponse(resp)
	}
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("cache explanation returned HTTP %d, expected synchronous 200", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

// AdmitLiquibaseCacheExplain admits one durable Liquibase explanation job.
// It deliberately does not retry an ambiguous POST: a lost response can mean
// the job was accepted, so the caller must consult the ordinary job list.
// See docs/architecture/liquibase-planning-recovery-design.md.
func (c *Client) AdmitLiquibaseCacheExplain(ctx context.Context, req PrepareJobRequest) (PrepareJobAccepted, error) {
	var accepted PrepareJobAccepted
	if req.PrepareKind != "lb" {
		return accepted, fmt.Errorf("durable cache explanation requires prepare_kind lb")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return accepted, err
	}
	resp, err := c.doRequestWithBody(ctx, http.MethodPost, "/v1/cache/explain/prepare", true, bytes.NewReader(body), "application/json")
	if err != nil {
		return accepted, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return accepted, parseErrorResponse(resp)
		}
		return accepted, fmt.Errorf("Liquibase cache explanation returned HTTP %d, expected 202", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		return PrepareJobAccepted{}, err
	}
	if accepted.JobID == "" || accepted.StatusURL == "" || resp.Header.Get("Location") != accepted.StatusURL {
		return PrepareJobAccepted{}, fmt.Errorf("Liquibase cache explanation returned an invalid job reference")
	}
	return accepted, nil
}

func (c *Client) ListNames(ctx context.Context, filters ListFilters) ([]NameEntry, error) {
	var out []NameEntry
	query := url.Values{}
	addFilter(query, "instance", filters.Instance)
	addFilter(query, "state", filters.State)
	addFilter(query, "image", filters.Image)
	if err := c.doJSON(ctx, http.MethodGet, appendQuery("/v1/names", query), true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetName(ctx context.Context, name string) (NameEntry, bool, error) {
	path := "/v1/names/" + url.PathEscape(strings.TrimSpace(name))
	var out NameEntry
	found, err := c.doJSONOptional(ctx, http.MethodGet, path, true, &out)
	return out, found, err
}

func (c *Client) ListInstances(ctx context.Context, filters ListFilters) ([]InstanceEntry, error) {
	var out []InstanceEntry
	query := url.Values{}
	addFilter(query, "state", filters.State)
	addFilter(query, "image", filters.Image)
	addFilter(query, "id_prefix", filters.IDPrefix)
	if err := c.doJSON(ctx, http.MethodGet, appendQuery("/v1/instances", query), true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetInstance(ctx context.Context, idOrName string) (InstanceEntry, bool, error) {
	path := "/v1/instances/" + url.PathEscape(strings.TrimSpace(idOrName))
	var out InstanceEntry
	found, err := c.doJSONOptional(ctx, http.MethodGet, path, true, &out)
	return out, found, err
}

func (c *Client) ListStates(ctx context.Context, filters ListFilters) ([]StateEntry, error) {
	var out []StateEntry
	query := url.Values{}
	addFilter(query, "kind", filters.Kind)
	addFilter(query, "image", filters.Image)
	addFilter(query, "id_prefix", filters.IDPrefix)
	if err := c.doJSON(ctx, http.MethodGet, appendQuery("/v1/states", query), true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetState(ctx context.Context, stateID string) (StateEntry, bool, error) {
	path := "/v1/states/" + url.PathEscape(strings.TrimSpace(stateID))
	var out StateEntry
	found, err := c.doJSONOptional(ctx, http.MethodGet, path, true, &out)
	return out, found, err
}

func (c *Client) DeleteInstance(ctx context.Context, instanceID string, opts DeleteOptions) (DeleteResult, int, error) {
	return c.deleteWithOptions(ctx, "/v1/instances/"+url.PathEscape(strings.TrimSpace(instanceID)), opts, false)
}

func (c *Client) DeleteState(ctx context.Context, stateID string, opts DeleteOptions) (DeleteResult, int, error) {
	return c.deleteWithOptions(ctx, "/v1/states/"+url.PathEscape(strings.TrimSpace(stateID)), opts, true)
}

func (c *Client) CreatePrepareJob(ctx context.Context, req PrepareJobRequest) (PrepareJobAccepted, error) {
	var out PrepareJobAccepted
	body, err := json.Marshal(req)
	if err != nil {
		return out, err
	}
	resp, err := c.doRequestWithBody(ctx, http.MethodPost, "/v1/prepare-jobs", true, bytes.NewReader(body), "application/json")
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		return out, parseErrorResponse(resp)
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) PutSourceBlob(ctx context.Context, digest string, body io.Reader) error {
	path := "/v1/source-blobs/sha256/" + url.PathEscape(strings.TrimSpace(digest))
	resp, err := c.doSourceRequestWithBody(ctx, http.MethodPut, path, true, body, "application/octet-stream")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return parseErrorResponse(resp)
	}
	return nil
}

func (c *Client) StreamPrepareEvents(ctx context.Context, eventsURL string, rangeHeader string) (*http.Response, error) {
	url := c.resolveURL(eventsURL)
	if url == "" {
		return nil, fmt.Errorf("prepare events url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	return c.http.Do(req)
}

func (c *Client) GetPrepareJob(ctx context.Context, jobID string) (PrepareJobStatus, bool, error) {
	path := "/v1/prepare-jobs/" + url.PathEscape(strings.TrimSpace(jobID))
	var out PrepareJobStatus
	found, err := c.doJSONOptional(ctx, http.MethodGet, path, true, &out)
	return out, found, err
}

func (c *Client) CancelPrepareJob(ctx context.Context, jobID string) (PrepareJobStatus, int, error) {
	path := "/v1/prepare-jobs/" + url.PathEscape(strings.TrimSpace(jobID)) + "/cancel"
	resp, err := c.doRequest(ctx, http.MethodPost, path, true)
	if err != nil {
		return PrepareJobStatus{}, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return PrepareJobStatus{}, resp.StatusCode, parseErrorResponse(resp)
	}

	var out PrepareJobStatus
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return PrepareJobStatus{}, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

func (c *Client) ListPrepareJobs(ctx context.Context, jobID string) ([]PrepareJobEntry, error) {
	var out []PrepareJobEntry
	query := url.Values{}
	addFilter(query, "job", jobID)
	if err := c.doJSON(ctx, http.MethodGet, appendQuery("/v1/prepare-jobs", query), true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) DeletePrepareJob(ctx context.Context, jobID string, opts DeleteOptions) (DeleteResult, int, error) {
	return c.deleteWithOptions(ctx, "/v1/prepare-jobs/"+url.PathEscape(strings.TrimSpace(jobID)), opts, false)
}

func (c *Client) ListTasks(ctx context.Context, jobID string) ([]TaskEntry, error) {
	var out []TaskEntry
	query := url.Values{}
	addFilter(query, "job", jobID)
	if err := c.doJSON(ctx, http.MethodGet, appendQuery("/v1/tasks", query), true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) RunCommand(ctx context.Context, req RunRequest) (io.ReadCloser, error) {
	var body []byte
	if req.Args == nil {
		req.Args = []string{}
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	body = data
	resp, err := c.doRequestWithBody(ctx, http.MethodPost, "/v1/runs", true, bytes.NewReader(body), "application/json")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, parseErrorResponse(resp)
	}
	return resp.Body, nil
}

func (c *Client) GetConfig(ctx context.Context, path string, effective bool) (any, error) {
	query := url.Values{}
	addFilter(query, "path", path)
	if effective {
		query.Set("effective", "true")
	}
	if strings.TrimSpace(path) == "" {
		var out map[string]any
		if err := c.doJSON(ctx, http.MethodGet, appendQuery("/v1/config", query), true, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	var out ConfigValue
	if err := c.doJSON(ctx, http.MethodGet, appendQuery("/v1/config", query), true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetConfigValue(ctx context.Context, path string, effective bool) (ConfigValue, error) {
	if strings.TrimSpace(path) == "" {
		return ConfigValue{}, errors.New("config path is required")
	}
	query := url.Values{}
	addFilter(query, "path", path)
	if effective {
		query.Set("effective", "true")
	}
	var out ConfigValue
	if err := c.doJSON(ctx, http.MethodGet, appendQuery("/v1/config", query), true, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) SetConfig(ctx context.Context, req ConfigValue) (ConfigValue, error) {
	var out ConfigValue
	body, err := json.Marshal(req)
	if err != nil {
		return out, err
	}
	resp, err := c.doRequestWithBody(ctx, http.MethodPatch, "/v1/config", true, bytes.NewReader(body), "application/json")
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, parseErrorResponse(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) RemoveConfig(ctx context.Context, path string) (ConfigValue, error) {
	var out ConfigValue
	query := url.Values{}
	addFilter(query, "path", path)
	resp, err := c.doRequest(ctx, http.MethodDelete, appendQuery("/v1/config", query), true)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, parseErrorResponse(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) GetConfigSchema(ctx context.Context) (any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/v1/config/schema", true, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, useAuth bool, out any) error {
	resp, err := c.doRequest(ctx, method, path, useAuth)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	decoder := json.NewDecoder(resp.Body)
	return decoder.Decode(out)
}

func (c *Client) doJSONOptional(ctx context.Context, method, path string, useAuth bool, out any) (bool, error) {
	resp, err := c.doRequest(ctx, method, path, useAuth)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	decoder := json.NewDecoder(resp.Body)
	return true, decoder.Decode(out)
}

func (c *Client) deleteWithOptions(ctx context.Context, path string, opts DeleteOptions, allowRecurse bool) (DeleteResult, int, error) {
	var out DeleteResult
	query := url.Values{}
	if opts.Force {
		query.Set("force", "true")
	}
	if opts.DryRun {
		query.Set("dry_run", "true")
	}
	if allowRecurse && opts.Recurse {
		query.Set("recurse", "true")
	}
	resp, err := c.doRequest(ctx, http.MethodDelete, appendQuery(path, query), true)
	if err != nil {
		return out, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return out, resp.StatusCode, parseErrorResponse(resp)
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&out); err != nil {
		return out, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, useAuth bool) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if useAuth && c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	return c.http.Do(req)
}

func (c *Client) doRequestWithBody(ctx context.Context, method, path string, useAuth bool, body io.Reader, contentType string) (*http.Response, error) {
	return c.doRequestWithBodyHeaders(ctx, method, path, useAuth, body, contentType, nil)
}

func (c *Client) doRequestWithBodyHeaders(ctx context.Context, method, path string, useAuth bool, body io.Reader, contentType string, headers map[string]string) (*http.Response, error) {
	return c.doRequestWithBodyHeadersUsing(c.http, ctx, method, path, useAuth, body, contentType, headers)
}

func (c *Client) doSourceRequestWithBody(ctx context.Context, method, path string, useAuth bool, body io.Reader, contentType string) (*http.Response, error) {
	httpClient := c.sourceHTTP
	if httpClient == nil {
		httpClient = c.http
	}
	return c.doRequestWithBodyHeadersUsing(httpClient, ctx, method, path, useAuth, body, contentType, nil)
}

func (c *Client) doRequestWithBodyHeadersUsing(httpClient *http.Client, ctx context.Context, method, path string, useAuth bool, body io.Reader, contentType string, headers map[string]string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if useAuth && c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	return httpClient.Do(req)
}

func normalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return value
	}
	if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		value = "http://" + value
	}
	return strings.TrimRight(value, "/")
}

func (c *Client) resolveURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.HasPrefix(value, "/") {
		return c.baseURL + value
	}
	return c.baseURL + "/" + value
}

type HTTPStatusError struct {
	StatusCode int
	Status     string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("unexpected status: %s", e.Status)
}

// ErrorResponseError preserves structured API error metadata while keeping
// Error() compatible with the previous message-only behavior.
type ErrorResponseError struct {
	StatusCode int
	Status     string
	Code       string
	Message    string
	Details    string
}

func (e *ErrorResponseError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("unexpected status: %s", e.Status)
	}
	if strings.TrimSpace(e.Details) != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Details)
	}
	return e.Message
}

// SourceInputsMissingError preserves the recoverable remote source-sync payload
// described in docs/architecture/remote-source-input-sync-flow.md.
type SourceInputsMissingError struct {
	StatusCode int
	Status     string
	Response   SourceInputsMissingErrorResponse
}

func (e *SourceInputsMissingError) Error() string {
	message := strings.TrimSpace(e.Response.Message)
	if message == "" {
		return fmt.Sprintf("unexpected status: %s", e.Status)
	}
	return message
}

func addFilter(values url.Values, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values.Set(key, value)
}

func appendQuery(path string, values url.Values) string {
	if len(values) == 0 {
		return path
	}
	return path + "?" + values.Encode()
}

func parseErrorResponse(resp *http.Response) error {
	if resp == nil {
		return &HTTPStatusError{StatusCode: 0, Status: "missing response"}
	}
	var errResp ErrorResponse
	data, _ := io.ReadAll(resp.Body)
	if len(data) > 0 {
		var missing SourceInputsMissingErrorResponse
		if resp.StatusCode == http.StatusConflict && json.Unmarshal(data, &missing) == nil && missing.Code == "source_inputs_missing" {
			return &SourceInputsMissingError{
				StatusCode: resp.StatusCode,
				Status:     resp.Status,
				Response:   missing,
			}
		}
		if json.Unmarshal(data, &errResp) == nil && errResp.Message != "" {
			return &ErrorResponseError{
				StatusCode: resp.StatusCode,
				Status:     resp.Status,
				Code:       errResp.Code,
				Message:    errResp.Message,
				Details:    errResp.Details,
			}
		}
		body := strings.TrimSpace(string(data))
		if body != "" {
			if len(body) > 200 {
				body = body[:200] + "..."
			}
			return fmt.Errorf("unexpected status: %s: %s", resp.Status, body)
		}
	}
	return &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
}
