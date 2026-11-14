package runtime

import (
	"context"
	"strings"
	"testing"
)

// R01/S02: a lost Docker create response is adopted only using the saved
// session/configuration/generation, never by the container name alone.
func TestLiquibaseSessionIdentityRoundTripsDockerLabels(t *testing.T) {
	id := strings.Repeat("a", 64)
	runner := &fakeRunner{responses: []runResponse{{output: id}, {output: id}, {output: `{"Id":"` + id + `","Name":"/sqlrs-lb-session-test","Config":{"Labels":{"sqlrs.liquibase.session":"session","sqlrs.liquibase.config":"config","sqlrs.liquibase.generation":"3","sqlrs.liquibase.helper":"helper","unrelated":"must-not-return"}},"State":{"Status":"created"}}`}}}
	rt := NewDocker(Options{Runner: runner})
	_, err := rt.CreateContainer(context.Background(), RunRequest{ImageID: "pinned", Name: "sqlrs-lb-session-test", Labels: map[string]string{"sqlrs.liquibase.session": "session", "sqlrs.liquibase.config": "config", "sqlrs.liquibase.generation": "3", "sqlrs.liquibase.helper": "helper"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"sqlrs.liquibase.session=session", "sqlrs.liquibase.config=config", "sqlrs.liquibase.generation=3", "sqlrs.liquibase.helper=helper"} {
		if !containsArg(runner.calls[0].args, "--label", label) {
			t.Fatalf("missing saved label %s", label)
		}
	}
	view, found, err := rt.InspectNamedContainer(context.Background(), "sqlrs-lb-session-test")
	if err != nil || !found || view.SessionID != "session" || view.RunnerConfigDigest != "config" || view.WorkerGeneration != "3" || view.CaptureHelperDigest != "helper" {
		t.Fatalf("saved labels missing: view=%+v found=%t err=%v", view, found, err)
	}
}
