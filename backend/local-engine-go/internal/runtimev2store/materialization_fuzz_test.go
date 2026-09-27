package runtimev2store

import (
	"encoding/json"
	"testing"
)

func FuzzCanonicalMetadata(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"a":[1,true,null]}`))
	f.Add([]byte(`{"a":1,"a":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		canonical, err := canonicalMetadata(json.RawMessage(raw))
		if err != nil {
			return
		}
		again, err := canonicalMetadata(canonical)
		if err != nil || string(again) != string(canonical) {
			t.Fatalf("canonical metadata is not idempotent: %q %q %v", canonical, again, err)
		}
	})
}
