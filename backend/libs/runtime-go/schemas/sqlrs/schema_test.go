package sqlrs_test

import (
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

func TestSupportedDockerBuilderExcludesOperationalMetadata(t *testing.T) {
	declaration := runtimev2.FactoryDeclaration{Kind: "docker", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{}}
	build := func(container string) runtimev2.CanonicalFingerprint {
		builder, err := sqlrs.NewDockerFactoryBuilder(declaration)
		if err != nil {
			t.Fatal(err)
		}
		image, _ := runtimev2.NewTextIdentityField("image", "postgres@sha256:abc")
		if err := builder.AddIdentityField(image); err != nil {
			t.Fatal(err)
		}
		observation, _ := runtimev2.NewOperationalObservation("sqlrs", "docker", "sqlrs.docker.observation.v1", map[string]string{"container_id": container})
		if err := builder.AddObservation(observation); err != nil {
			t.Fatal(err)
		}
		composition, err := builder.Build()
		if err != nil {
			t.Fatal(err)
		}
		fingerprint, err := runtimev2.CanonicalFactoryFingerprint(composition.Identity())
		if err != nil {
			t.Fatal(err)
		}
		return fingerprint
	}
	if build("one") != build("two") {
		t.Fatal("operational container ID changed identity")
	}
}
