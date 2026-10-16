package sqlrs

import (
	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// executionEncoder maps the reviewed schema through public canonical helpers.
// It never reads/writes framing or hashes. Requirements: structure §8–9.
type executionEncoder struct{ err error }
type executionPair struct {
	key   string
	value runtimev2.CanonicalValue
}

func ep(key string, value runtimev2.CanonicalValue) executionPair { return executionPair{key, value} }
func (e *executionEncoder) text(value string) runtimev2.CanonicalValue {
	// Raw preflight and literal normalization establish this constructor's
	// UTF-8/string-size preconditions; field/key constants are bounded too.
	v, _ := runtimev2.CanonicalString(value)
	return v
}
func (e *executionEncoder) object(pairs ...executionPair) runtimev2.CanonicalValue {
	if e.err != nil {
		return runtimev2.CanonicalValue{}
	}
	nodes := 1 + len(pairs)
	for _, pair := range pairs {
		if pair.value.NodeCount() > runtimev2.MaxCanonicalValueNodes-nodes {
			e.err = executionError(runtimev2.CodeLimitExceeded, "value")
			return runtimev2.CanonicalValue{}
		}
		nodes += pair.value.NodeCount()
	}
	entries := make([]runtimev2.CanonicalMapEntry, len(pairs))
	for i, pair := range pairs {
		entries[i] = runtimev2.CanonicalMapEntry{Key: pair.key, Value: pair.value}
	}
	v, err := runtimev2.CanonicalMap(entries)
	if err != nil {
		e.err = err
	}
	return v
}
func (e *executionEncoder) list(values []runtimev2.CanonicalValue) runtimev2.CanonicalValue {
	if e.err != nil {
		return runtimev2.CanonicalValue{}
	}
	nodes := 1
	for _, v := range values {
		if v.NodeCount() > runtimev2.MaxCanonicalValueNodes-nodes {
			e.err = executionError(runtimev2.CodeLimitExceeded, "value")
			return runtimev2.CanonicalValue{}
		}
		nodes += v.NodeCount()
	}
	v, err := runtimev2.CanonicalList(values)
	if err != nil {
		e.err = err
	}
	return v
}
func (e *executionEncoder) path(v ExecutionPath) runtimev2.CanonicalValue {
	return e.object(ep("root", e.text(v.Root)), ep("relative", e.text(v.Relative)))
}
func (e *executionEncoder) word(v ExecutionText) runtimev2.CanonicalValue {
	parts := make([]runtimev2.CanonicalValue, len(v))
	for i, p := range v {
		fields := []executionPair{ep("kind", e.text(string(p.Kind)))}
		switch p.Kind {
		case ExecutionTextPartLiteral:
			fields = append(fields, ep("literal", e.text(p.Literal)))
		case ExecutionTextPartSecret:
			fields = append(fields, ep("slot", e.text(p.SecretSlot)))
		case ExecutionTextPartService:
			fields = append(fields, ep("role", e.text(p.ServiceRole)), ep("field", e.text(string(p.ServiceField))))
		case ExecutionTextPartPath:
			fields = append(fields, ep("path", e.path(*p.Path)), ep("format", e.text(string(p.PathFormat))))
		}
		parts[i] = e.object(fields...)
	}
	return e.list(parts)
}

func executionFields(v ExecutionEnvironmentInput) ([]runtimev2.IdentityField, error) {
	e := &executionEncoder{}
	runtimeFields := []executionPair{ep("identity_kind", e.text(string(v.Runtime.IdentityKind))), ep("digest", e.text(v.Runtime.IdentityDigest)), ep("platform", e.object(ep("os", e.text(v.Runtime.Platform.OS)), ep("architecture", e.text(v.Runtime.Platform.Architecture)), ep("variant", e.text(v.Runtime.Platform.Variant))))}
	if v.Runtime.Kind == ExecutionRuntimeDatabaseContainer {
		runtimeFields = append(runtimeFields, ep("database_service", e.text(v.Runtime.DatabaseService)))
	}
	argv := make([]runtimev2.CanonicalValue, len(v.Argv))
	for i, w := range v.Argv {
		argv[i] = e.word(w)
	}
	environment := make([]executionPair, len(v.Environment))
	for i, variable := range v.Environment {
		environment[i] = ep(variable.Name, e.word(variable.Value))
	}
	inputs := make([]executionPair, len(v.Inputs))
	for i, input := range v.Inputs {
		fields := []executionPair{ep("source", e.object(ep("provider", e.text(input.Source.Provider)), ep("kind", e.text(input.Source.Kind)), ep("identity_schema", e.text(input.Source.IdentitySchema)), ep("fingerprint", e.text(string(input.Source.Fingerprint))))), ep("profile", e.text(string(input.Profile)))}
		if input.Profile == ExecutionInputRegularFile {
			flag := "no"
			if input.Executable {
				flag = "yes"
			}
			fields = append(fields, ep("executable", e.text(flag)))
		}
		inputs[i] = ep(input.Name, e.object(fields...))
	}
	services := make([]executionPair, len(v.Services))
	for i, service := range v.Services {
		fields := []executionPair{ep("protocol", e.text(service.Protocol)), ep("database", e.text(service.Database)), ep("user", e.text(service.User)), ep("transport", e.text(string(service.Transport)))}
		if service.AuthenticationSecret != "" {
			fields = append(fields, ep("authentication_secret", e.text(service.AuthenticationSecret)), ep("authentication_profile", e.text(service.AuthenticationProfile)))
		}
		services[i] = ep(service.Role, e.object(fields...))
	}
	mounts := make([]runtimev2.CanonicalValue, len(v.Mounts))
	for i, mount := range v.Mounts {
		access := "read-write"
		if mount.ReadOnly {
			access = "read-only"
		}
		fields := []executionPair{ep("name", e.text(mount.Name)), ep("target", e.path(mount.Target)), ep("kind", e.text(string(mount.Kind))), ep("access", e.text(access))}
		if mount.Kind == ExecutionMountInput {
			fields = append(fields, ep("input_name", e.text(mount.InputName)))
		}
		if mount.Kind == ExecutionMountSecret {
			fields = append(fields, ep("secret_slot", e.text(mount.SecretSlot)))
		}
		mounts[i] = e.object(fields...)
	}
	secrets := make([]executionPair, len(v.Secrets))
	for i, secret := range v.Secrets {
		fields := []executionPair{ep("purpose", e.text(string(secret.Purpose)))}
		if secret.Purpose == ExecutionSecretAuthentication {
			fields = append(fields, ep("service_role", e.text(secret.ServiceRole)))
		} else {
			ref := secret.IdentityReference
			fields = append(fields, ep("reference", e.object(ep("provider", e.text(ref.Provider())), ep("identifier", e.text(ref.Identifier())), ep("version", e.text(ref.Version())))))
		}
		secrets[i] = ep(secret.Name, e.object(fields...))
	}
	roots := []executionPair{
		ep("runtime", e.object(runtimeFields...)),
		ep("invocation", e.object(ep("argv", e.list(argv)), ep("environment", e.object(environment...)), ep("workdir", e.path(v.Workdir)), ep("user", e.text(v.User)))),
		ep("inputs", e.object(inputs...)), ep("network", e.object(ep("mode", e.text(string(v.Network))), ep("services", e.object(services...)))), ep("mounts", e.list(mounts)), ep("secrets", e.object(secrets...)),
	}
	if e.err != nil {
		return nil, e.err
	}
	nodes, size := 0, len(v.Runtime.Kind)
	for _, root := range roots {
		if root.value.NodeCount() > runtimev2.MaxCanonicalValueNodes-nodes || root.value.EncodedSize() > runtimev2.MaxCanonicalValueBytes-size {
			return nil, executionError(runtimev2.CodeLimitExceeded, "fields")
		}
		nodes += root.value.NodeCount()
		size += root.value.EncodedSize()
	}
	primitive, _ := runtimev2.NewTextIdentityField("primitive", string(v.Runtime.Kind))
	result := []runtimev2.IdentityField{primitive}
	for _, root := range roots {
		// Names are fixed schema constants; e.err==nil proves each opaque
		// value was built successfully. The root constructor's two input
		// preconditions are therefore established, not supplied by a caller.
		field, _ := runtimev2.NewCanonicalValueIdentityField(root.key, root.value)
		result = append(result, field)
	}
	return result, nil
}

// executionReader performs bounded closed-shape reads via immutable root views.
// It preserves the first static-path failure and never uses a caller key in errors.
type executionReader struct{ err error }

func (r *executionReader) fail(code runtimev2.ValidationCode, path string) {
	if r.err == nil {
		r.err = executionError(code, path)
	}
}
func (r *executionReader) text(v runtimev2.CanonicalValue, path string) string {
	if r.err != nil {
		return ""
	}
	text, ok := v.StringValue()
	if !ok {
		r.fail(runtimev2.CodeShapeInvalid, path)
	}
	return text
}
func (r *executionReader) entries(v runtimev2.CanonicalValue, path string) []runtimev2.CanonicalMapEntry {
	if r.err != nil {
		return nil
	}
	entries, ok := v.Entries()
	if !ok {
		r.fail(runtimev2.CodeShapeInvalid, path)
	}
	return entries
}
func (r *executionReader) list(v runtimev2.CanonicalValue, path string) []runtimev2.CanonicalValue {
	if r.err != nil {
		return nil
	}
	if v.Kind() != runtimev2.CanonicalValueKindList {
		r.fail(runtimev2.CodeShapeInvalid, path)
		return nil
	}
	values, _ := v.Elements()
	return values
}
func (r *executionReader) object(v runtimev2.CanonicalValue, path string, required []string, optional ...string) map[string]runtimev2.CanonicalValue {
	entries := r.entries(v, path)
	result := make(map[string]runtimev2.CanonicalValue, len(entries))
	for _, entry := range entries {
		allowed := false
		for _, name := range required {
			if entry.Key == name {
				allowed = true
			}
		}
		for _, name := range optional {
			if entry.Key == name {
				allowed = true
			}
		}
		if !allowed {
			r.fail(runtimev2.CodeUnknownMember, path)
			return result
		}
		result[entry.Key] = entry.Value
	}
	for _, name := range required {
		if _, ok := result[name]; !ok {
			r.fail(runtimev2.CodeShapeInvalid, path)
		}
	}
	return result
}
func (r *executionReader) path(v runtimev2.CanonicalValue, path string) ExecutionPath {
	fields := r.object(v, path, []string{"root", "relative"})
	return ExecutionPath{Root: r.text(fields["root"], path), Relative: r.text(fields["relative"], path)}
}
func (r *executionReader) word(v runtimev2.CanonicalValue, path string) ExecutionText {
	values := r.list(v, path)
	result := make(ExecutionText, len(values))
	for i, value := range values {
		pPath := executionIndex(path, i)
		// Read the discriminator without accepting any extra variant members.
		var kind string
		for _, entry := range r.entries(value, pPath) {
			if entry.Key == "kind" {
				kind = r.text(entry.Value, pPath)
			}
		}
		keys := []string{"kind"}
		switch ExecutionTextPartKind(kind) {
		case ExecutionTextPartLiteral:
			keys = append(keys, "literal")
		case ExecutionTextPartSecret:
			keys = append(keys, "slot")
		case ExecutionTextPartService:
			keys = append(keys, "role", "field")
		case ExecutionTextPartPath:
			keys = append(keys, "path", "format")
		default:
			r.fail(runtimev2.CodeValueInvalid, pPath)
		}
		fields := r.object(value, pPath, keys)
		part := ExecutionTextPart{Kind: ExecutionTextPartKind(kind)}
		switch part.Kind {
		case ExecutionTextPartLiteral:
			part.Literal = r.text(fields["literal"], pPath)
		case ExecutionTextPartSecret:
			part.SecretSlot = r.text(fields["slot"], pPath)
		case ExecutionTextPartService:
			part.ServiceRole = r.text(fields["role"], pPath)
			part.ServiceField = ExecutionServiceField(r.text(fields["field"], pPath))
		case ExecutionTextPartPath:
			p := r.path(fields["path"], pPath)
			part.Path = &p
			part.PathFormat = ExecutionPathFormat(r.text(fields["format"], pPath))
		}
		result[i] = part
	}
	return result
}

func executionRead(identity runtimev2.CanonicalResolvedExtensionIdentity) (ExecutionEnvironmentInput, error) {
	fields := identity.Fields()
	if len(fields) != 7 {
		return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeShapeInvalid, "fields")
	}
	r := &executionReader{}
	forest := map[string]runtimev2.CanonicalValue{}
	v := ExecutionEnvironmentInput{}
	nodes, size := 0, 0
	for _, field := range fields {
		name := field.Field.Name()
		if name == "primitive" {
			if field.Field.Kind() != runtimev2.IdentityFieldText || field.Disclosure != runtimev2.DisclosurePublic {
				return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeShapeInvalid, "fields")
			}
			v.Runtime.Kind = ExecutionRuntimeKind(field.Field.Text())
			size += len(v.Runtime.Kind)
			continue
		}
		switch name {
		case "runtime", "invocation", "inputs", "network", "mounts", "secrets":
		default:
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeUnknownMember, "fields")
		}
		if field.Field.Kind() != runtimev2.IdentityFieldCanonicalValue || field.Disclosure != runtimev2.DisclosureProtected {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeShapeInvalid, "fields")
		}
		value := field.Field.CanonicalValue()
		if value.NodeCount() > runtimev2.MaxCanonicalValueNodes-nodes || value.EncodedSize() > runtimev2.MaxCanonicalValueBytes-size {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeLimitExceeded, "fields")
		}
		nodes += value.NodeCount()
		size += value.EncodedSize()
		forest[name] = value
	}
	if !forest["runtime"].Valid() || v.Runtime.Kind == "" {
		return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeShapeInvalid, "fields")
	}
	runtimeKeys := []string{"identity_kind", "digest", "platform"}
	if v.Runtime.Kind == ExecutionRuntimeDatabaseContainer {
		runtimeKeys = append(runtimeKeys, "database_service")
	}
	runtimeFields := r.object(forest["runtime"], "runtime", runtimeKeys)
	v.Runtime.IdentityKind = ExecutionRuntimeIdentityKind(r.text(runtimeFields["identity_kind"], "runtime.identity_kind"))
	v.Runtime.IdentityDigest = r.text(runtimeFields["digest"], "runtime.digest")
	if v.Runtime.Kind == ExecutionRuntimeDatabaseContainer {
		v.Runtime.DatabaseService = r.text(runtimeFields["database_service"], "runtime.database_service")
	}
	platform := r.object(runtimeFields["platform"], "runtime.platform", []string{"os", "architecture", "variant"})
	v.Runtime.Platform = ExecutionPlatform{OS: r.text(platform["os"], "runtime.platform"), Architecture: r.text(platform["architecture"], "runtime.platform"), Variant: r.text(platform["variant"], "runtime.platform")}
	invocation := r.object(forest["invocation"], "invocation", []string{"argv", "environment", "workdir", "user"})
	for i, w := range r.list(invocation["argv"], "argv") {
		v.Argv = append(v.Argv, r.word(w, executionIndex("argv", i)))
	}
	for i, e := range r.entries(invocation["environment"], "environment") {
		v.Environment = append(v.Environment, ExecutionEnvironmentVariable{Name: e.Key, Value: r.word(e.Value, executionIndex("environment", i))})
	}
	v.Workdir = r.path(invocation["workdir"], "workdir")
	v.User = r.text(invocation["user"], "user")
	for i, e := range r.entries(forest["inputs"], "inputs") {
		path := executionIndex("inputs", i)
		fields := r.object(e.Value, path, []string{"source", "profile"}, "executable")
		profile := ExecutionInputProfile(r.text(fields["profile"], path))
		source := r.object(fields["source"], path, []string{"provider", "kind", "identity_schema", "fingerprint"})
		input := ExecutionInputBinding{Name: e.Key, Profile: profile, Source: ExecutionInputReference{Provider: r.text(source["provider"], path), Kind: r.text(source["kind"], path), IdentitySchema: r.text(source["identity_schema"], path), Fingerprint: runtimev2.CanonicalFingerprint(r.text(source["fingerprint"], path))}}
		if profile == ExecutionInputRegularFile {
			flag := r.text(fields["executable"], path)
			if flag != "yes" && flag != "no" {
				r.fail(runtimev2.CodeValueInvalid, path)
			}
			input.Executable = flag == "yes"
		} else if _, present := fields["executable"]; present {
			r.fail(runtimev2.CodeUnknownMember, path)
		}
		v.Inputs = append(v.Inputs, input)
	}
	network := r.object(forest["network"], "network", []string{"mode", "services"})
	v.Network = ExecutionNetworkMode(r.text(network["mode"], "network.mode"))
	for i, e := range r.entries(network["services"], "services") {
		path := executionIndex("services", i)
		fields := r.object(e.Value, path, []string{"protocol", "database", "user", "transport"}, "authentication_secret", "authentication_profile")
		service := ExecutionServiceBinding{Role: e.Key, Protocol: r.text(fields["protocol"], path), Database: r.text(fields["database"], path), User: r.text(fields["user"], path), Transport: ExecutionServiceTransport(r.text(fields["transport"], path))}
		if value, present := fields["authentication_secret"]; present {
			service.AuthenticationSecret = r.text(value, path)
		}
		if value, present := fields["authentication_profile"]; present {
			service.AuthenticationProfile = r.text(value, path)
		}
		v.Services = append(v.Services, service)
	}
	for i, e := range r.list(forest["mounts"], "mounts") {
		path := executionIndex("mounts", i)
		fields := r.object(e, path, []string{"name", "target", "kind", "access"}, "input_name", "secret_slot")
		mount := ExecutionMount{Name: r.text(fields["name"], path), Target: r.path(fields["target"], path), Kind: ExecutionMountKind(r.text(fields["kind"], path))}
		access := r.text(fields["access"], path)
		if access != "read-only" && access != "read-write" {
			r.fail(runtimev2.CodeValueInvalid, path)
		}
		mount.ReadOnly = access == "read-only"
		if value, present := fields["input_name"]; present {
			mount.InputName = r.text(value, path)
			if mount.Kind != ExecutionMountInput {
				r.fail(runtimev2.CodeUnknownMember, path)
			}
		}
		if value, present := fields["secret_slot"]; present {
			mount.SecretSlot = r.text(value, path)
			if mount.Kind != ExecutionMountSecret {
				r.fail(runtimev2.CodeUnknownMember, path)
			}
		}
		v.Mounts = append(v.Mounts, mount)
	}
	for i, e := range r.entries(forest["secrets"], "secrets") {
		path := executionIndex("secrets", i)
		fields := r.object(e.Value, path, []string{"purpose"}, "service_role", "reference")
		purpose := ExecutionSecretPurpose(r.text(fields["purpose"], path))
		secret := ExecutionSecretBinding{Name: e.Key, Purpose: purpose}
		if purpose == ExecutionSecretAuthentication {
			secret.ServiceRole = r.text(fields["service_role"], path)
			if _, present := fields["reference"]; present {
				r.fail(runtimev2.CodeUnknownMember, path)
			}
		} else if purpose == ExecutionSecretSemantic {
			if _, present := fields["service_role"]; present {
				r.fail(runtimev2.CodeUnknownMember, path)
			}
			refFields := r.object(fields["reference"], path, []string{"provider", "identifier", "version"})
			provider, identifier, version := r.text(refFields["provider"], path), r.text(refFields["identifier"], path), r.text(refFields["version"], path)
			ref, err := runtimev2.NewSecretReference(provider, identifier, version)
			if err != nil {
				r.fail(runtimev2.CodeValueInvalid, path)
			}
			secret.IdentityReference = &ref
		} else {
			r.fail(runtimev2.CodeValueInvalid, path)
		}
		v.Secrets = append(v.Secrets, secret)
	}
	if r.err != nil {
		return ExecutionEnvironmentInput{}, r.err
	}
	return v, nil
}
