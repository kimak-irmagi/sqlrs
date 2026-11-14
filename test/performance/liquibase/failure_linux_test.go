package benchmark

import "testing"

func TestFailureClassificationNeverReturnsPrivateText(t *testing.T) {
	cases := []struct {
		input *ErrorResponse
		want  string
	}{
		{nil, "unknown"},
		{&ErrorResponse{Message: "PRIVATE_SENTINEL", Details: "postgres://PRIVATE_SENTINEL"}, "unknown"},
		{&ErrorResponse{Message: "cannot initialize base state"}, "base_state"},
		{&ErrorResponse{Message: "cannot parse liquibase changesets"}, "plan_parse"},
		{&ErrorResponse{Message: "snapshot failed"}, "snapshot"},
		{&ErrorResponse{Message: "cannot start runtime"}, "runtime_start"},
		{&ErrorResponse{Message: "cannot clone state"}, "clone_state"},
		{&ErrorResponse{Message: "cannot prepare scripts"}, "scripts_prepare"},
		{&ErrorResponse{Message: "liquibase execution failed", Details: "Connection refused PRIVATE_SENTINEL"}, "connection_refused"},
		{&ErrorResponse{Message: "liquibase execution failed", Details: "No suitable driver PRIVATE_SENTINEL"}, "jdbc_missing"},
		{&ErrorResponse{Message: "liquibase execution failed", Details: "Permission denied PRIVATE_SENTINEL"}, "permission_denied"},
		{&ErrorResponse{Message: "liquibase execution failed", Details: "The file PRIVATE_SENTINEL was not found"}, "input_missing"},
		{&ErrorResponse{Message: "liquibase execution failed", Details: "No such file or directory PRIVATE_SENTINEL"}, "file_missing"},
		{&ErrorResponse{Message: "liquibase execution failed", Details: "Unexpected argument PRIVATE_SENTINEL"}, "cli_arguments"},
		{&ErrorResponse{Message: "liquibase execution failed", Details: "PRIVATE_SENTINEL"}, "liquibase_execute"},
	}
	for _, item := range cases {
		if got := classifyEngineFailure(item.input); got != item.want {
			t.Fatalf("classification=%s want=%s", got, item.want)
		}
	}
}
