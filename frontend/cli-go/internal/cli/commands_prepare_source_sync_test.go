package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/inputset"
	"github.com/sqlrs/cli/internal/remotesource"
)

func TestCreatePrepareJobWithSourceSyncRetriesAndUploads(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "query.sql")
	if err := os.WriteFile(sourcePath, []byte("select 1;\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	hash := "sha256:" + inputset.HashContent([]byte("select 1;\n"))
	digest := strings.TrimPrefix(hash, "sha256:")
	postCount := 0
	putCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs":
			postCount++
			var req client.PrepareJobRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode prepare request: %v", err)
			}
			if req.SourceManifest == nil {
				t.Fatal("expected source_manifest on remote source-sync request")
			}
			if postCount == 1 {
				w.WriteHeader(http.StatusConflict)
				io.WriteString(w, `{"code":"source_inputs_missing","message":"missing","missing_blobs":[{"path":"query.sql","hash":"`+hash+`"}]}`)
				return
			}
			if got := req.SourceManifest.Files["query.sql"]; got != hash {
				t.Fatalf("manifest hash = %q, want %q", got, hash)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1"}`)
		case "/v1/source-blobs/sha256/" + digest:
			putCount++
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read blob: %v", err)
			}
			if string(data) != "select 1;\n" {
				t.Fatalf("uploaded body = %q", string(data))
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	accepted, err := createPrepareJobWithSourceSync(
		context.Background(),
		client.New(server.URL, client.Options{}),
		PrepareOptions{SourceSync: &remotesource.Options{
			Enabled:       true,
			MaxRounds:     2,
			WorkspaceRoot: root,
		}},
		client.PrepareJobRequest{PrepareKind: "psql", ImageID: "postgres:16"},
	)
	if err != nil {
		t.Fatalf("createPrepareJobWithSourceSync: %v", err)
	}
	if accepted.JobID != "job-1" || postCount != 2 || putCount != 1 {
		t.Fatalf("accepted=%+v postCount=%d putCount=%d", accepted, postCount, putCount)
	}
}
