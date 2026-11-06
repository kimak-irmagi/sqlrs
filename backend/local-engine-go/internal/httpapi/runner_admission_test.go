package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sqlrs/engine-local/internal/config"
	"github.com/sqlrs/engine-local/internal/prepare"
	"github.com/sqlrs/engine-local/internal/registry"
	"github.com/sqlrs/engine-local/internal/store/sqlite"
)

// TestStrictRunnerHTTPAdmission covers presence, not just nonempty decoded
// values, at both public request boundaries in the approved OpenAPI design.
func TestStrictRunnerHTTPAdmission(t *testing.T) {
	server := runnerAdmissionServer(t, nil)
	values := map[string][]string{
		"liquibase_exec":      {`"client"`, `""`, `null`},
		"liquibase_exec_mode": {`"native"`, `""`, `null`},
		"liquibase_env":       {`{"JAVA_HOME":"client"}`, `{}`, `null`},
	}
	for _, endpoint := range []string{"/v1/prepare-jobs", "/v1/cache/explain/prepare"} {
		for key, cases := range values {
			for _, value := range cases {
				t.Run(endpoint+"/"+key+"/"+value, func(t *testing.T) {
					body := `{"prepare_kind":"lb","image_id":"image-1","liquibase_args":["update"],"` + key + `":` + value + `}`
					status, code := postRunnerAdmission(t, server.URL+endpoint, body)
					if status != http.StatusBadRequest || code != "invalid_argument" {
						t.Fatalf("status=%d code=%q", status, code)
					}
				})
			}
		}
	}
}

// TestStrictRunnerHTTPAdversarialJSON ensures decoder key normalization,
// duplicate keys, and kind ambiguity cannot bypass the admission check.
func TestStrictRunnerHTTPAdversarialJSON(t *testing.T) {
	server := runnerAdmissionServer(t, nil)
	cases := []string{
		`{"prepare_kind":"lb","image_id":"image-1","Liquibase_Exec":""}`,
		`{"prepare_kind":"lb","image_id":"image-1","liquibase_\u0065xec":null}`,
		`{"prepare_kind":"lb","image_id":"image-1","liquibase_exec":"client","liquibase_exec":""}`,
		`{"prepare_kind":"psql","prepare_kind":"lb","image_id":"image-1","liquibase_env":null}`,
		`{"prepare_kind":"lb","PREPARE_KIND":"psql","image_id":"image-1","liquibase_exec_mode":""}`,
	}
	for _, endpoint := range []string{"/v1/prepare-jobs", "/v1/cache/explain/prepare"} {
		for _, body := range cases {
			t.Run(endpoint+"/"+body, func(t *testing.T) {
				status, code := postRunnerAdmission(t, server.URL+endpoint, body)
				if status != http.StatusBadRequest || code != "invalid_argument" {
					t.Fatalf("status=%d code=%q", status, code)
				}
			})
		}
	}
}

// TestStrictRunnerKindBoundary applies the runner rule even to psql payloads.
func TestStrictRunnerKindBoundary(t *testing.T) {
	server := runnerAdmissionServer(t, nil)
	for _, endpoint := range []string{"/v1/prepare-jobs", "/v1/cache/explain/prepare"} {
		for _, suffix := range []string{`"liquibase_exec":""`, `"liquibase_exec_mode":null`, `"liquibase_env":{}`} {
			body := `{"prepare_kind":"psql","image_id":"image-1","psql_args":["-c","select 1"],` + suffix + `}`
			status, code := postRunnerAdmission(t, server.URL+endpoint, body)
			if status != http.StatusBadRequest || code != "invalid_argument" {
				t.Fatalf("%s: status=%d code=%q", endpoint, status, code)
			}
		}
	}
}

// TestStrictRunnerAdmissionIgnoresPlanAndManifest proves these request fields
// cannot upgrade a client into a trusted runner configuration source.
func TestStrictRunnerAdmissionIgnoresPlanAndManifest(t *testing.T) {
	server := runnerAdmissionServer(t, nil)
	for _, endpoint := range []string{"/v1/prepare-jobs", "/v1/cache/explain/prepare"} {
		for _, body := range []string{
			`{"prepare_kind":"lb","image_id":"image-1","liquibase_args":["update"],"plan_only":true,"liquibase_exec":null}`,
			`{"prepare_kind":"lb","image_id":"image-1","liquibase_args":["update"],"plan_only":false,"source_manifest":{"files":{}},"liquibase_exec":""}`,
		} {
			status, code := postRunnerAdmission(t, server.URL+endpoint, body)
			if status != http.StatusBadRequest || code != "invalid_argument" {
				t.Fatalf("%s: status=%d code=%q", endpoint, status, code)
			}
		}
	}
}

// TestStrictRunnerAdmissionAllowsCleanKinds ensures the rule is limited to
// runner ownership and does not block ordinary Liquibase or psql jobs.
func TestStrictRunnerAdmissionAllowsCleanKinds(t *testing.T) {
	server := runnerAdmissionServer(t, nil)
	for _, body := range []string{
		`{"prepare_kind":"lb","image_id":"image-1","liquibase_args":["update"],"plan_only":true}`,
		`{"prepare_kind":"psql","image_id":"image-1","psql_args":["-c","select 1"],"plan_only":true}`,
	} {
		status, code := postRunnerAdmission(t, server.URL+"/v1/prepare-jobs", body)
		if status != http.StatusCreated {
			t.Fatalf("clean request rejected: status=%d code=%q", status, code)
		}
	}
}

// TestLiquibaseVersionAdmission covers the public selector without allowing a
// client to choose a runner repository or executable.
func TestLiquibaseVersionAdmission(t *testing.T) {
	for _, version := range []string{"latest", "5.0.3"} {
		t.Run("valid/"+version, func(t *testing.T) {
			server := runnerAdmissionServer(t, nil)
			body := `{"prepare_kind":"lb","image_id":"image-1","liquibase_args":["update"],"plan_only":true,"liquibase_version":"` + version + `"}`
			status, code := postRunnerAdmission(t, server.URL+"/v1/prepare-jobs", body)
			if status != http.StatusCreated {
				t.Fatalf("valid version rejected: status=%d code=%q", status, code)
			}
		})
	}
	for _, version := range []string{"", "liquibase/liquibase:latest", "sha256:abc", "../latest"} {
		for _, endpoint := range []string{"/v1/prepare-jobs", "/v1/cache/explain/prepare"} {
			t.Run("invalid/"+version+endpoint, func(t *testing.T) {
				var calls atomic.Int32
				server := runnerAdmissionServer(t, func(options *prepare.Options) {
					options.Liquibase = runnerAdmissionLiquibaseStub{calls: &calls}
				})
				body := `{"prepare_kind":"lb","image_id":"image-1","liquibase_args":["update"],"plan_only":true,"liquibase_version":"` + version + `"}`
				status, code := postRunnerAdmission(t, server.URL+endpoint, body)
				if calls.Load() != 0 {
					t.Fatalf("invalid version invoked Liquibase %d times", calls.Load())
				}
				if status != http.StatusBadRequest || code != "invalid_argument" {
					t.Fatalf("invalid version accepted: status=%d code=%q", status, code)
				}
			})
		}
	}
}

// The OpenAPI selector is a string field on lb only. Explicit null and a
// psql request cannot silently fall back to the default tag.
func TestLiquibaseVersionAdmissionRejectsNullAndWrongKind(t *testing.T) {
	server := runnerAdmissionServer(t, nil)
	for _, endpoint := range []string{"/v1/prepare-jobs", "/v1/cache/explain/prepare"} {
		for _, body := range []string{
			`{"prepare_kind":"lb","image_id":"image-1","liquibase_args":["update"],"liquibase_version":null}`,
			`{"prepare_kind":"psql","image_id":"image-1","psql_args":["-c","select 1"],"liquibase_version":"5.0.3"}`,
		} {
			status, code := postRunnerAdmission(t, server.URL+endpoint, body)
			if status != http.StatusBadRequest || code != "invalid_argument" {
				t.Fatalf("%s accepted selector for wrong shape: status=%d code=%q body=%s", endpoint, status, code, body)
			}
		}
	}
}

// TestLiquibaseUnknownFieldAdmission keeps both public request decoders strict
// before source synchronization, image resolution, or job creation.
func TestLiquibaseUnknownFieldAdmission(t *testing.T) {
	for _, endpoint := range []string{"/v1/prepare-jobs", "/v1/cache/explain/prepare"} {
		t.Run(endpoint, func(t *testing.T) {
			var calls atomic.Int32
			server := runnerAdmissionServer(t, func(options *prepare.Options) {
				options.Liquibase = runnerAdmissionLiquibaseStub{calls: &calls}
			})
			body := `{"prepare_kind":"lb","image_id":"image-1","liquibase_args":["update"],"liquibase_image":"other/liquibase:latest"}`
			status, code := postRunnerAdmission(t, server.URL+endpoint, body)
			if calls.Load() != 0 {
				t.Fatalf("unknown field invoked Liquibase %d times", calls.Load())
			}
			if status != http.StatusBadRequest || code != "invalid_argument" {
				t.Fatalf("unknown field accepted: status=%d code=%q", status, code)
			}
		})
	}
}

// TestPublicConfigCannotReadOrSetTrustedRunner verifies the public config
// resource is not a control plane for service-owned executable selection.
func TestPublicConfigCannotReadOrSetTrustedRunner(t *testing.T) {
	cfg, err := config.NewManager(config.Options{StateStoreRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(Options{AuthToken: "secret", Config: cfg})
	for _, path := range []string{"/v1/config", "/v1/config/schema"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer secret")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK || strings.Contains(resp.Body.String(), "trusted-liquibase") || strings.Contains(resp.Body.String(), "runnerConfig") {
			t.Fatalf("config disclosure at %s: %d %s", path, resp.Code, resp.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPatch, "/v1/config", strings.NewReader(`{"path":"liquibase.exec","value":"client-liquibase"}`))
	req.Header.Set("Authorization", "Bearer secret")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code < 400 {
		t.Fatalf("public config accepted runner mutation: %d %s", resp.Code, resp.Body.String())
	}
}

func runnerAdmissionServer(t *testing.T, option func(*prepare.Options)) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	st, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	q := mustOpenQueue(t, dbPath)
	if option == nil {
		option = func(*prepare.Options) {}
	}
	prep := newPrepareManager(t, st, q, func(options *prepare.Options) {
		options.Liquibase = runnerAdmissionLiquibaseStub{}
		option(options)
	})
	server := httptest.NewServer(NewHandler(Options{Version: "test", InstanceID: "instance", AuthToken: "secret", Registry: registry.New(st), Prepare: prep}))
	t.Cleanup(server.Close)
	return server
}

// Admission tests must never invoke the host's Liquibase executable, even if
// the current implementation mistakenly accepts a request that should fail.
type runnerAdmissionLiquibaseStub struct{ calls *atomic.Int32 }

func (stub runnerAdmissionLiquibaseStub) Run(context.Context, prepare.LiquibaseRunRequest) (string, error) {
	if stub.calls != nil {
		stub.calls.Add(1)
	}
	return "", nil
}

func postRunnerAdmission(t *testing.T, endpoint, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(bytes.NewReader(data)).Decode(&payload)
	return resp.StatusCode, payload.Code
}
