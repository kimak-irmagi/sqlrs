package resolver

import (
	"encoding/json"
	"testing"
)

func FuzzDecodeCacheRecordJSON(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"schema_version":"sqlrs.resolution-cache.canonical.v1"}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		record, err := DecodeCacheRecordJSON(raw, coverageSchema())
		if err != nil {
			return
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("accepted record no longer marshals: %v", err)
		}
		if _, err := DecodeCacheRecordJSON(encoded, coverageSchema()); err != nil {
			t.Fatalf("accepted record did not round trip: %v", err)
		}
	})
}
