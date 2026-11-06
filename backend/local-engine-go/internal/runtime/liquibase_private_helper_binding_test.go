package runtime

import (
	"bytes"
	"os"
	"testing"
)

// PC04: new sessions pin the tested private/v2 bytes, not the historical v1
// helper. Returning copies prevents callers from mutating future bindings.
func TestPrivateSessionHelperPinsV2(t *testing.T) {
	want, err := os.ReadFile("assets/liquibase-step-capture-v2.sh")
	if err != nil {
		t.Fatal(err)
	}
	got, digest := LiquibaseStepCaptureHelper()
	if !bytes.Equal(got, want) || digest != captureDigest(want) {
		t.Fatal("new session pins historical/noncanonical helper")
	}
	got[0] ^= 1
	again, againDigest := LiquibaseStepCaptureHelper()
	if !bytes.Equal(again, want) || againDigest != digest {
		t.Fatal("caller mutated retained helper")
	}
}
