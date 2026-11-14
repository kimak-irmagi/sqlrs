package benchmark

import (
	"reflect"
	"testing"
)

func TestOracleMismatchReportsDimensionsNotValues(t *testing.T) {
	expected := "change|1|id|author|config/master.xml|checksum||\ncolumn|public|x"
	actual := []string{"change|1|id|author|PRIVATE_SENTINEL|checksum||", "column|public|x"}
	if got := oracleMismatchParts(actual, expected); !reflect.DeepEqual(got, []string{"path"}) {
		t.Fatal(got)
	}
	if got := oracleMismatchParts([]string{"column|private"}, expected); !reflect.DeepEqual(got, []string{"row_count"}) {
		t.Fatal(got)
	}
	if got := oracleMismatchParts([]string{"change|1|id|author|config/master.xml|checksum||", "column|public|changed"}, expected); !reflect.DeepEqual(got, []string{"schema"}) {
		t.Fatal(got)
	}
}
