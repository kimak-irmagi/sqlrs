package consumertest

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// DL07 is copied into both the isolated source consumer and release workflow.
// It intentionally depends on the exported constant and constructors, not on
// checkout-local paths, replacement modules or private encoding helpers.
func TestDeclarationValueLimitReleaseConsumer(t *testing.T) {
	if runtimev2.MaxDeclarationValueBytes != 256<<10 || runtimev2.MaxResolvedValueBytes != 4096 {
		t.Fatal("incorrect public bounds")
	}
	for _, role := range []string{"input", "execution_environment", "deployment"} {
		t.Run(role, func(t *testing.T) {
			input := runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "source", SpecificationSchema: "example.source.v1", Fields: []runtimev2.DeclarationField{{Name: "payload", Value: strings.Repeat("x", 256<<10)}}}
			construct := func() (runtimev2.ExtensionDeclaration, error) {
				switch role {
				case "input":
					return runtimev2.NewInputDeclaration(input)
				case "execution_environment":
					return runtimev2.NewExecutionEnvironmentDeclaration(input)
				default:
					return runtimev2.NewDeploymentDeclaration(input)
				}
			}
			value, err := construct()
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var decoded runtimev2.ExtensionDeclaration
			switch role {
			case "input":
				var v runtimev2.InputDeclaration
				err = runtimev2.DecodeJSON(raw, &v)
				decoded = v
			case "execution_environment":
				var v runtimev2.ExecutionEnvironmentDeclaration
				err = runtimev2.DecodeJSON(raw, &v)
				decoded = v
			default:
				var v runtimev2.DeploymentDeclaration
				err = runtimev2.DecodeJSON(raw, &v)
				decoded = v
			}
			if err != nil || decoded.Fields()[0].Value != input.Fields[0].Value {
				t.Fatalf("payload round trip: %v", err)
			}
			input.Fields[0].Value += "x"
			_, err = construct()
			var problem *runtimev2.ValidationError
			if !errors.Is(err, runtimev2.ErrInvalid) || !errors.As(err, &problem) || problem.Code != runtimev2.CodeTooLarge || problem.Path != "fields[0].value" {
				t.Fatalf("wrong overflow error: %v", err)
			}
		})
	}
	if _, err := runtimev2.CanonicalString(strings.Repeat("x", 4097)); err == nil {
		t.Fatal("canonical value limit changed")
	}
	for _, n := range []int{4096, 4097} {
		input := runtimev2.DiagnosticObservationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "observation", ObservationSchema: "example.observation.v1", Fields: []runtimev2.DeclarationField{{Name: "note", Value: strings.Repeat("x", n)}}}
		_, capabilityErr := runtimev2.NewCapabilityObservation(input)
		_, portabilityErr := runtimev2.NewPortabilityObservation(input)
		for _, err := range []error{capabilityErr, portabilityErr} {
			if n == 4096 && err != nil {
				t.Fatal(err)
			}
			if n == 4097 {
				var problem *runtimev2.ValidationError
				if !errors.As(err, &problem) || problem.Code != runtimev2.CodeTooLarge || problem.Path != "fields[0].value" {
					t.Fatalf("diagnostic bound changed: %v", err)
				}
			}
		}
	}
}
