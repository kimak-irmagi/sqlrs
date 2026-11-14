package benchmark

import (
	"strconv"
	"strings"
	"testing"
)

// UR01/UR02: each resumed suffix is validated after exact association. A prefix
// match cannot admit corrupt arguments/results or substitute another call.
func TestDiagnosticRawResumeRejectsCorruption(t *testing.T) {
	pending := "2 100.1 " + rawTestEntry + " <unfinished ...>\n"
	for index, tail := range []string{
		"", ")", ") =", ") = 0 junk", ") = 1", ") = -1", ") = 0 <0..1>",
		"0) = 0", ", 0) = 0", ") = 0\r", ") = ? <unavailable> junk",
	} {
		t.Run("resume_"+strconv.Itoa(index), func(t *testing.T) {
			assertRawRejected(t, pending+"2 100.2 <... syscall_0xabc resumed>"+tail+"\n"+rawTestExit)
		})
	}
	for _, entry := range []string{
		"2 100.0 execve(\"/bin/true\", [], 0x0 <unfinished ...>\n",
		"2 100.0 ???(0,0,0,0,0,0) = ?\n",
		"2 100.0 ???(0,0,0,0,0,0) = ? <unavailable>\n",
	} {
		assertRawRejected(t, entry+"2 100.2 <... ??? resumed>) = 0\n"+rawTestExit)
	}
	for _, suffix := range []string{"0 <9223372037.0>", "0 <18446744073709551616.0>"} {
		assertRawRejected(t, "2 100.1 "+rawTestEntry+") = "+suffix+"\n"+rawTestExit)
	}
	// A full maximum-size line cannot evade the existing stream quotas through
	// the new grammar. The private message/number is never copied to errors.
	assertRawRejected(t, "2 100.1 syscall_0xabc("+strings.Repeat("0", 65536)+") = 0\n"+rawTestExit)
}
