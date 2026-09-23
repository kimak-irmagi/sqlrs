package runtimev2

import (
	"encoding/json"
	"sort"
)

// FactoryDeclaration records unresolved, non-identity-bearing factory input.
type FactoryDeclaration struct {
	Kind                 string                           `json:"kind"`
	Reference            string                           `json:"reference"`
	Arguments            []string                         `json:"arguments"`
	Attributes           map[string]string                `json:"attributes"`
	Inputs               []InputDeclaration               `json:"inputs,omitempty"`
	ExecutionEnvironment *ExecutionEnvironmentDeclaration `json:"execution_environment,omitempty"`
	Deployment           *DeploymentDeclaration           `json:"deployment,omitempty"`
	Capabilities         []CapabilityObservation          `json:"capabilities,omitempty"`
	Portability          []PortabilityObservation         `json:"portability,omitempty"`
}

// TransformDeclaration records unresolved, non-identity-bearing transform input.
type TransformDeclaration struct {
	Kind                 string                           `json:"kind"`
	Reference            string                           `json:"reference"`
	Arguments            []string                         `json:"arguments"`
	Attributes           map[string]string                `json:"attributes"`
	Inputs               []InputDeclaration               `json:"inputs,omitempty"`
	ExecutionEnvironment *ExecutionEnvironmentDeclaration `json:"execution_environment,omitempty"`
	Capabilities         []CapabilityObservation          `json:"capabilities,omitempty"`
	Portability          []PortabilityObservation         `json:"portability,omitempty"`
}

func validateFactoryDeclaration(value *FactoryDeclaration) error {
	if value == nil {
		return invalid(CodeInvalidShape, "$")
	}
	if err := validateDeclaration(value.Kind, value.Reference, value.Arguments, value.Attributes); err != nil {
		return err
	}
	if err := validateDeclarationExtensions(value.Inputs, value.ExecutionEnvironment, value.Capabilities, value.Portability); err != nil {
		return err
	}
	if value.Deployment != nil && value.Deployment.data == nil {
		return invalid(CodeInvalidShape, "deployment")
	}
	return nil
}

func validateTransformDeclaration(value *TransformDeclaration) error {
	if value == nil {
		return invalid(CodeInvalidShape, "$")
	}
	if err := validateDeclaration(value.Kind, value.Reference, value.Arguments, value.Attributes); err != nil {
		return err
	}
	return validateDeclarationExtensions(value.Inputs, value.ExecutionEnvironment, value.Capabilities, value.Portability)
}

func validateDeclarationExtensions(inputs []InputDeclaration, environment *ExecutionEnvironmentDeclaration, capabilities []CapabilityObservation, portability []PortabilityObservation) error {
	if len(inputs) > MaxResolvedFields {
		return invalid(CodeTooLarge, "inputs")
	}
	for index, input := range inputs {
		if input.data == nil {
			return invalid(CodeInvalidShape, "inputs["+itoa(index)+"]")
		}
	}
	if environment != nil && environment.data == nil {
		return invalid(CodeInvalidShape, "execution_environment")
	}
	if len(capabilities) > MaxResolvedFields {
		return invalid(CodeTooLarge, "capabilities")
	}
	for index, value := range capabilities {
		if value.data == nil {
			return invalid(CodeInvalidShape, "capabilities["+itoa(index)+"]")
		}
	}
	if len(portability) > MaxResolvedFields {
		return invalid(CodeTooLarge, "portability")
	}
	for index, value := range portability {
		if value.data == nil {
			return invalid(CodeInvalidShape, "portability["+itoa(index)+"]")
		}
	}
	return nil
}

// ResolverObservation records non-identity-bearing resolver diagnostics.
type ResolverObservation struct {
	Implementation string `json:"implementation"`
	Version        string `json:"version"`
}

func validateDeclaration(kind, reference string, arguments []string, attributes map[string]string) error {
	if err := validateIdentifier(kind, "kind"); err != nil {
		return err
	}
	if err := validateUTF8(reference, "reference", false, MaxResolvedValueBytes); err != nil {
		return err
	}
	if len(arguments) > MaxArguments {
		return invalid(CodeTooLarge, "arguments")
	}
	for index, argument := range arguments {
		if err := validateUTF8(argument, "arguments["+itoa(index)+"]", false, MaxResolvedValueBytes); err != nil {
			return err
		}
	}
	if len(attributes) > MaxAttributes {
		return invalid(CodeTooLarge, "attributes")
	}
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := validateIdentifier(key, "attributes."+key); err != nil {
			return err
		}
		if err := validateUTF8(attributes[key], "attributes."+key, true, MaxResolvedValueBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateResolver(value *ResolverObservation) error {
	if value == nil {
		return nil
	}
	if err := validateIdentifier(value.Implementation, "implementation"); err != nil {
		return err
	}
	return validateUTF8(value.Version, "version", false, MaxIdentifierBytes)
}

func copyAttributes(source map[string]string) map[string]string {
	if source == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func copyStrings(source []string) []string {
	result := make([]string, len(source))
	copy(result, source)
	return result
}

func copyFactoryDeclaration(source *FactoryDeclaration) *FactoryDeclaration {
	if source == nil {
		return nil
	}
	result := &FactoryDeclaration{Kind: source.Kind, Reference: source.Reference, Arguments: copyStrings(source.Arguments), Attributes: copyAttributes(source.Attributes)}
	result.Inputs = copyInputDeclarations(source.Inputs)
	result.ExecutionEnvironment = copyExecutionEnvironment(source.ExecutionEnvironment)
	result.Deployment = copyDeployment(source.Deployment)
	result.Capabilities = copyCapabilities(source.Capabilities)
	result.Portability = copyPortability(source.Portability)
	return result
}

func copyTransformDeclaration(source *TransformDeclaration) *TransformDeclaration {
	if source == nil {
		return nil
	}
	result := &TransformDeclaration{Kind: source.Kind, Reference: source.Reference, Arguments: copyStrings(source.Arguments), Attributes: copyAttributes(source.Attributes)}
	result.Inputs = copyInputDeclarations(source.Inputs)
	result.ExecutionEnvironment = copyExecutionEnvironment(source.ExecutionEnvironment)
	result.Capabilities = copyCapabilities(source.Capabilities)
	result.Portability = copyPortability(source.Portability)
	return result
}

func copyInputDeclarations(source []InputDeclaration) []InputDeclaration {
	result := make([]InputDeclaration, len(source))
	for index := range source {
		result[index] = InputDeclaration{data: source[index].data.clone()}
	}
	return result
}
func copyExecutionEnvironment(source *ExecutionEnvironmentDeclaration) *ExecutionEnvironmentDeclaration {
	if source == nil {
		return nil
	}
	return &ExecutionEnvironmentDeclaration{data: source.data.clone()}
}
func copyDeployment(source *DeploymentDeclaration) *DeploymentDeclaration {
	if source == nil {
		return nil
	}
	return &DeploymentDeclaration{data: source.data.clone()}
}
func copyCapabilities(source []CapabilityObservation) []CapabilityObservation {
	result := make([]CapabilityObservation, len(source))
	for index := range source {
		result[index] = CapabilityObservation{data: source[index].data.clone()}
	}
	return result
}
func copyPortability(source []PortabilityObservation) []PortabilityObservation {
	result := make([]PortabilityObservation, len(source))
	for index := range source {
		result[index] = PortabilityObservation{data: source[index].data.clone()}
	}
	return result
}

func copyResolver(source *ResolverObservation) *ResolverObservation {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

type declarationWire struct {
	Kind                 *string                          `json:"kind"`
	Reference            *string                          `json:"reference"`
	Arguments            *[]string                        `json:"arguments"`
	Attributes           *map[string]string               `json:"attributes"`
	Inputs               []InputDeclaration               `json:"inputs,omitempty"`
	ExecutionEnvironment *ExecutionEnvironmentDeclaration `json:"execution_environment,omitempty"`
	Deployment           *DeploymentDeclaration           `json:"deployment,omitempty"`
	Capabilities         []CapabilityObservation          `json:"capabilities,omitempty"`
	Portability          []PortabilityObservation         `json:"portability,omitempty"`
}

func declarationFromWire(w declarationWire) (string, string, []string, map[string]string, error) {
	if w.Kind == nil {
		return "", "", nil, nil, invalid(CodeInvalidShape, "kind")
	}
	if w.Reference == nil {
		return "", "", nil, nil, invalid(CodeInvalidShape, "reference")
	}
	if w.Arguments == nil {
		return "", "", nil, nil, invalid(CodeInvalidShape, "arguments")
	}
	if w.Attributes == nil {
		return "", "", nil, nil, invalid(CodeInvalidShape, "attributes")
	}
	if err := validateDeclaration(*w.Kind, *w.Reference, *w.Arguments, *w.Attributes); err != nil {
		return "", "", nil, nil, err
	}
	return *w.Kind, *w.Reference, copyStrings(*w.Arguments), copyAttributes(*w.Attributes), nil
}

func (d FactoryDeclaration) MarshalJSON() ([]byte, error) {
	if err := validateFactoryDeclaration(&d); err != nil {
		return nil, err
	}
	args, attrs := copyStrings(d.Arguments), copyAttributes(d.Attributes)
	return json.Marshal(declarationWire{Kind: &d.Kind, Reference: &d.Reference, Arguments: &args, Attributes: &attrs, Inputs: copyInputDeclarations(d.Inputs), ExecutionEnvironment: copyExecutionEnvironment(d.ExecutionEnvironment), Deployment: copyDeployment(d.Deployment), Capabilities: copyCapabilities(d.Capabilities), Portability: copyPortability(d.Portability)})
}
func (d *FactoryDeclaration) UnmarshalJSON(data []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire declarationWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	kind, ref, args, attrs, err := declarationFromWire(wire)
	if err != nil {
		return err
	}
	value := FactoryDeclaration{Kind: kind, Reference: ref, Arguments: args, Attributes: attrs, Inputs: wire.Inputs, ExecutionEnvironment: wire.ExecutionEnvironment, Deployment: wire.Deployment, Capabilities: wire.Capabilities, Portability: wire.Portability}
	if err := validateFactoryDeclaration(&value); err != nil {
		return err
	}
	*d = *copyFactoryDeclaration(&value)
	return nil
}
func (d TransformDeclaration) MarshalJSON() ([]byte, error) {
	if err := validateTransformDeclaration(&d); err != nil {
		return nil, err
	}
	args, attrs := copyStrings(d.Arguments), copyAttributes(d.Attributes)
	return json.Marshal(declarationWire{Kind: &d.Kind, Reference: &d.Reference, Arguments: &args, Attributes: &attrs, Inputs: copyInputDeclarations(d.Inputs), ExecutionEnvironment: copyExecutionEnvironment(d.ExecutionEnvironment), Capabilities: copyCapabilities(d.Capabilities), Portability: copyPortability(d.Portability)})
}
func (d *TransformDeclaration) UnmarshalJSON(data []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire declarationWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	kind, ref, args, attrs, err := declarationFromWire(wire)
	if err != nil {
		return err
	}
	if wire.Deployment != nil {
		return invalid(CodeInvalidShape, "deployment")
	}
	value := TransformDeclaration{Kind: kind, Reference: ref, Arguments: args, Attributes: attrs, Inputs: wire.Inputs, ExecutionEnvironment: wire.ExecutionEnvironment, Capabilities: wire.Capabilities, Portability: wire.Portability}
	if err := validateTransformDeclaration(&value); err != nil {
		return err
	}
	*d = *copyTransformDeclaration(&value)
	return nil
}
