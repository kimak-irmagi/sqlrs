package oldconsumertest

import (
	"os"
	"testing"

	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
)

// CR07: this file is compiled in isolation against the immutable v0.4.0
// module. The old decoder must never accept the v0.5.0 typed cache record.
func TestOldDecoderRejectsCanonicalCacheRecord(t *testing.T) {
	raw, err := os.ReadFile("new-cache-record.json")
	if err != nil { t.Fatal(err) }
	if _, err := resolver.DecodeCacheRecordJSON(raw); err == nil { t.Fatal("old decoder accepted canonical cache record") }
}
