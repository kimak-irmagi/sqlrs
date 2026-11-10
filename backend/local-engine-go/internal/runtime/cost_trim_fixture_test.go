package runtime

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"
)

// presentInspection supplies only the full-ID observation authorized by
// OT-CT01. Discovery fixtures retain their independent list+inspect contract.
func presentInspection(id, name, status string, exit int) []runResponse {
	return accessInspection(id, name, status, exit)[1:]
}

// sessionBundleFixture changes only the acquisition unit. Missing files and
// non-regular headers remain missing/non-regular rather than fabricated proof.
func sessionBundleFixture(t *testing.T, dir, id, invocation string) map[string][]byte {
	t.Helper()
	individual := privateCaptureArchives(t, dir, id, invocation)
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	root := id + ":/sqlrs/capture/" + invocation + "/"
	for _, name := range []string{"manifest.complete", "stdout.complete", "stderr.complete"} {
		data, ok := individual[root+name]
		if !ok {
			continue
		}
		r := tar.NewReader(bytes.NewReader(data))
		h, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(w, r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{root + ".": buf.Bytes()}
}
