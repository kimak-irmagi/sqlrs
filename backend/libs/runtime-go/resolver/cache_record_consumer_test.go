package resolver_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCacheRecordStandaloneConsumerCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("standalone module smoke test")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate module")
	}
	moduleRoot := filepath.Dir(filepath.Dir(source))
	consumer := t.TempDir()
	goMod := "module example.com/cache-record-consumer\n\ngo 1.25.0\n\nrequire github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go v0.0.0\nreplace github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go => " + filepath.ToSlash(moduleRoot) + "\n"
	program := `package consumer
import "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
var _ = resolver.NewCacheRecord
var _ = resolver.DecodeCacheRecordJSON
func consume(record resolver.CacheRecord, key resolver.CacheKey) resolver.Resolution {
    _ = record.Matches(key)
    _, _ = record.MarshalJSON()
    return record.Resolution()
}
`
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(consumer, "consumer.go"), []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "./...")
	command.Dir = consumer
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("standalone consumer: %v\n%s", err, output)
	}
}
