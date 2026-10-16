package sqlrs

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

var (
	executionName            = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
	executionIdentifier      = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	executionPlatformAtom    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	executionEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	executionDigestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	executionProfileVersion  = regexp.MustCompile(`\.v[1-9][0-9]*$`)
)

// executionError never incorporates caller-controlled map keys or payloads.
func executionError(code runtimev2.ValidationCode, path string) error {
	return &runtimev2.ValidationError{Code: code, Path: path}
}
func executionIndex(path string, index int) string { return path + "[" + strconv.Itoa(index) + "]" }

// executionPreflight bounds raw input before proportional copy/sort/merge.
// Requirements: execution-schema-structure §9 and tests ES17. Checked subtraction
// prevents integer overflow, and normalization cannot hide redundant raw parts.
func executionPreflight(v ExecutionEnvironmentInput) error {
	total := 0
	text := func(path string, values ...string) error {
		for _, s := range values {
			if len(s) > runtimev2.MaxCanonicalStringBytes || len(s) > runtimev2.MaxCanonicalValueBytes-total {
				return executionError(runtimev2.CodeLimitExceeded, path)
			}
			if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
				return executionError(runtimev2.CodeValueInvalid, path)
			}
			total += len(s)
		}
		return nil
	}
	count := func(n int, path string) error {
		if n > runtimev2.MaxCanonicalCollectionMembers {
			return executionError(runtimev2.CodeLimitExceeded, path)
		}
		return nil
	}
	for _, c := range []struct {
		n    int
		path string
	}{{len(v.Argv), "argv"}, {len(v.Environment), "environment"}, {len(v.Inputs), "inputs"}, {len(v.Mounts), "mounts"}, {len(v.Services), "services"}, {len(v.Secrets), "secrets"}} {
		if err := count(c.n, c.path); err != nil {
			return err
		}
	}
	if err := text("runtime", string(v.Runtime.Kind), string(v.Runtime.IdentityKind), v.Runtime.IdentityDigest, v.Runtime.Platform.OS, v.Runtime.Platform.Architecture, v.Runtime.Platform.Variant, v.Runtime.DatabaseService, v.User, string(v.Network), v.Workdir.Root, v.Workdir.Relative); err != nil {
		return err
	}
	word := func(value ExecutionText, path string) error {
		if err := count(len(value), path); err != nil {
			return err
		}
		for i, p := range value {
			pPath := executionIndex(path, i)
			if err := text(pPath, string(p.Kind), p.Literal, p.SecretSlot, p.ServiceRole, string(p.ServiceField), string(p.PathFormat)); err != nil {
				return err
			}
			if p.Path != nil {
				if err := text(pPath, p.Path.Root, p.Path.Relative); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for i, w := range v.Argv {
		if err := word(w, executionIndex("argv", i)); err != nil {
			return err
		}
	}
	for i, e := range v.Environment {
		path := executionIndex("environment", i)
		if err := text(path, e.Name); err != nil {
			return err
		}
		if err := word(e.Value, path); err != nil {
			return err
		}
	}
	for i, e := range v.Inputs {
		if err := text(executionIndex("inputs", i), e.Name, e.Source.Provider, e.Source.Kind, e.Source.IdentitySchema, string(e.Source.Fingerprint), string(e.Profile)); err != nil {
			return err
		}
	}
	for i, e := range v.Mounts {
		if err := text(executionIndex("mounts", i), e.Name, e.InputName, e.Target.Root, e.Target.Relative, string(e.Kind), e.SecretSlot); err != nil {
			return err
		}
	}
	for i, e := range v.Services {
		if err := text(executionIndex("services", i), e.Role, e.Protocol, e.Database, e.User, string(e.Transport), e.AuthenticationSecret, e.AuthenticationProfile); err != nil {
			return err
		}
	}
	for i, e := range v.Secrets {
		path := executionIndex("secrets", i)
		if err := text(path, e.Name, string(e.Purpose), e.ServiceRole); err != nil {
			return err
		}
		if e.IdentityReference != nil {
			if err := text(path, e.IdentityReference.Provider(), e.IdentityReference.Identifier(), e.IdentityReference.Version()); err != nil {
				return err
			}
		}
	}
	return nil
}

// executionValidation owns bounded per-call indexes and declared-use tracking.
// It proves structural references, never physical paths or provider capabilities.
type executionValidation struct {
	v                                  ExecutionEnvironmentInput
	inputs, mounts, services, secrets  map[string]int
	inputUsed, serviceUsed, secretUsed map[string]bool
}

func executionNormalize(input ExecutionEnvironmentInput) (ExecutionEnvironmentInput, error) {
	v := input
	v.Inputs = append([]ExecutionInputBinding(nil), input.Inputs...)
	v.Mounts = append([]ExecutionMount(nil), input.Mounts...)
	v.Services = append([]ExecutionServiceBinding(nil), input.Services...)
	v.Secrets = append([]ExecutionSecretBinding(nil), input.Secrets...)
	v.Environment = append([]ExecutionEnvironmentVariable(nil), input.Environment...)
	v.Argv = make([]ExecutionText, len(input.Argv))
	state := executionValidation{v: v, inputs: map[string]int{}, mounts: map[string]int{}, services: map[string]int{}, secrets: map[string]int{}, inputUsed: map[string]bool{}, serviceUsed: map[string]bool{}, secretUsed: map[string]bool{}}
	add := func(index map[string]int, name, path string, i int) error {
		if !executionName.MatchString(name) {
			return executionError(runtimev2.CodeValueInvalid, path)
		}
		if _, exists := index[name]; exists {
			return executionError(runtimev2.CodeNonCanonical, path)
		}
		index[name] = i
		return nil
	}
	for i, e := range v.Inputs {
		if err := add(state.inputs, e.Name, executionIndex("inputs", i), i); err != nil {
			return ExecutionEnvironmentInput{}, err
		}
	}
	for i, e := range v.Mounts {
		if err := add(state.mounts, e.Name, executionIndex("mounts", i), i); err != nil {
			return ExecutionEnvironmentInput{}, err
		}
	}
	for i, e := range v.Services {
		if err := add(state.services, e.Role, executionIndex("services", i), i); err != nil {
			return ExecutionEnvironmentInput{}, err
		}
	}
	for i, e := range v.Secrets {
		if err := add(state.secrets, e.Name, executionIndex("secrets", i), i); err != nil {
			return ExecutionEnvironmentInput{}, err
		}
	}
	if err := state.runtime(); err != nil {
		return ExecutionEnvironmentInput{}, err
	}
	for i, e := range v.Inputs {
		path := executionIndex("inputs", i)
		if !executionIdentifier.MatchString(e.Source.Provider) || !executionIdentifier.MatchString(e.Source.Kind) || !executionIdentifier.MatchString(e.Source.IdentitySchema) || !executionDigestPattern.MatchString(string(e.Source.Fingerprint)) {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
		}
		if e.Profile != ExecutionInputRegularFile && e.Profile != ExecutionInputRegularTree || e.Profile == ExecutionInputRegularTree && e.Executable {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
		}
	}
	for i, e := range v.Services {
		if err := state.service(e, executionIndex("services", i)); err != nil {
			return ExecutionEnvironmentInput{}, err
		}
	}
	for i, e := range v.Secrets {
		path := executionIndex("secrets", i)
		switch e.Purpose {
		case ExecutionSecretSemantic:
			if e.ServiceRole != "" || e.IdentityReference == nil || !e.IdentityReference.Valid() {
				return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
			}
			// Valid references are opaque and immutable: their only public
			// constructor already validated every component. Preflight adds this
			// facade's NUL/raw-budget rules. Copy the handle to a new pointer;
			// rebuilding it cannot establish any additional requirement.
			ref := *e.IdentityReference
			v.Secrets[i].IdentityReference = &ref
		case ExecutionSecretAuthentication:
			service, exists := state.services[e.ServiceRole]
			if e.IdentityReference != nil || !exists || v.Services[service].AuthenticationSecret != e.Name {
				return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
			}
			state.serviceUsed[e.ServiceRole] = true
			state.secretUsed[e.Name] = true
		default:
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
		}
	}
	targets := map[ExecutionPath]bool{}
	for i, e := range v.Mounts {
		path := executionIndex("mounts", i)
		if err := state.path(e.Target, false, true, path); err != nil {
			return ExecutionEnvironmentInput{}, err
		}
		if targets[e.Target] {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeNonCanonical, path)
		}
		targets[e.Target] = true
		switch e.Kind {
		case ExecutionMountInput:
			if _, ok := state.inputs[e.InputName]; !ok || e.SecretSlot != "" {
				return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
			}
			state.inputUsed[e.InputName] = true
		case ExecutionMountScratch:
			if e.InputName != "" || e.SecretSlot != "" || e.ReadOnly {
				return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
			}
		case ExecutionMountSecret:
			secret, ok := state.secrets[e.SecretSlot]
			if !ok || v.Secrets[secret].Purpose != ExecutionSecretSemantic || e.InputName != "" || !e.ReadOnly {
				return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
			}
			state.secretUsed[e.SecretSlot] = true
		default:
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
		}
	}
	if err := state.path(v.Workdir, true, false, "workdir"); err != nil {
		return ExecutionEnvironmentInput{}, err
	}
	if len(input.Argv) == 0 {
		return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, "argv")
	}
	for i, w := range input.Argv {
		normalized, err := state.word(w, false, executionIndex("argv", i))
		if err != nil {
			return ExecutionEnvironmentInput{}, err
		}
		v.Argv[i] = normalized
	}
	first := v.Argv[0]
	if len(first) != 1 || first[0].Kind != ExecutionTextPartLiteral && first[0].Kind != ExecutionTextPartPath || first[0].Kind == ExecutionTextPartLiteral && first[0].Literal == "" {
		return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, "argv[0]")
	}
	if v.Runtime.Kind == ExecutionRuntimeHost && (first[0].Kind != ExecutionTextPartPath || first[0].Path.Root != "tool" || first[0].PathFormat != ExecutionPathNative) {
		return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, "argv[0]")
	}
	names := map[string]bool{}
	for i, e := range v.Environment {
		path := executionIndex("environment", i)
		if !executionEnvironmentName.MatchString(e.Name) {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, path)
		}
		if v.Runtime.Platform.OS == "windows" {
			e.Name = strings.ToUpper(e.Name)
		}
		if names[e.Name] {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeNonCanonical, path)
		}
		names[e.Name] = true
		normalized, err := state.word(e.Value, true, path)
		if err != nil {
			return ExecutionEnvironmentInput{}, err
		}
		v.Environment[i] = ExecutionEnvironmentVariable{Name: e.Name, Value: normalized}
	}
	for i, e := range v.Inputs {
		if !state.inputUsed[e.Name] {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, executionIndex("inputs", i))
		}
	}
	for i, e := range v.Services {
		if !state.serviceUsed[e.Role] {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, executionIndex("services", i))
		}
	}
	for i, e := range v.Secrets {
		if !state.secretUsed[e.Name] {
			return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeValueInvalid, executionIndex("secrets", i))
		}
	}
	sort.Slice(v.Inputs, func(i, j int) bool { return v.Inputs[i].Name < v.Inputs[j].Name })
	sort.Slice(v.Services, func(i, j int) bool { return v.Services[i].Role < v.Services[j].Role })
	sort.Slice(v.Secrets, func(i, j int) bool { return v.Secrets[i].Name < v.Secrets[j].Name })
	sort.Slice(v.Environment, func(i, j int) bool { return v.Environment[i].Name < v.Environment[j].Name })
	return v, nil
}

func (s *executionValidation) runtime() error {
	v := s.v
	if !executionDigestPattern.MatchString(v.Runtime.IdentityDigest) || !executionPlatformAtom.MatchString(v.Runtime.Platform.OS) || !executionPlatformAtom.MatchString(v.Runtime.Platform.Architecture) || v.Runtime.Platform.Variant != "" && !executionPlatformAtom.MatchString(v.Runtime.Platform.Variant) {
		return executionError(runtimev2.CodeValueInvalid, "runtime")
	}
	switch v.Runtime.Kind {
	case ExecutionRuntimeHost:
		if v.Runtime.IdentityKind != ExecutionIdentityHostBundle && v.Runtime.IdentityKind != ExecutionIdentityHostExecutable || v.User == "" {
			return executionError(runtimev2.CodeValueInvalid, "runtime")
		}
	case ExecutionRuntimeOCI, ExecutionRuntimeDatabaseContainer:
		if v.Runtime.IdentityKind != ExecutionIdentityImageManifest {
			return executionError(runtimev2.CodeValueInvalid, "runtime")
		}
	default:
		return executionError(runtimev2.CodeValueInvalid, "runtime")
	}
	if v.Runtime.Kind == ExecutionRuntimeDatabaseContainer {
		if _, ok := s.services[v.Runtime.DatabaseService]; !ok {
			return executionError(runtimev2.CodeValueInvalid, "runtime.database_service")
		}
		s.serviceUsed[v.Runtime.DatabaseService] = true
	} else if v.Runtime.DatabaseService != "" {
		return executionError(runtimev2.CodeValueInvalid, "runtime.database_service")
	}
	switch v.Network {
	case ExecutionNetworkNone:
	case ExecutionNetworkServices:
		for role := range s.services {
			s.serviceUsed[role] = true
		}
	case ExecutionNetworkLocalContext:
		if v.Runtime.Kind != ExecutionRuntimeHost || v.Runtime.IdentityKind != ExecutionIdentityHostExecutable {
			return executionError(runtimev2.CodeValueInvalid, "network")
		}
	default:
		return executionError(runtimev2.CodeValueInvalid, "network")
	}
	return nil
}

func (s *executionValidation) service(v ExecutionServiceBinding, path string) error {
	if !executionIdentifier.MatchString(v.Protocol) || v.Transport != ExecutionServiceTCP && v.Transport != ExecutionServiceUnixSocket {
		return executionError(runtimev2.CodeValueInvalid, path)
	}
	if v.AuthenticationSecret == "" && v.AuthenticationProfile == "" {
		return nil
	}
	secret, ok := s.secrets[v.AuthenticationSecret]
	if !ok || s.v.Secrets[secret].Purpose != ExecutionSecretAuthentication || s.v.Secrets[secret].ServiceRole != v.Role || v.User == "" || !executionIdentifier.MatchString(v.AuthenticationProfile) || !executionProfileVersion.MatchString(v.AuthenticationProfile) {
		return executionError(runtimev2.CodeValueInvalid, path)
	}
	return nil
}

// path validates logical coordinates only; physical confinement is owner I/O.
func (s *executionValidation) path(v ExecutionPath, directory, target bool, path string) error {
	bad := func() error { return executionError(runtimev2.CodeValueInvalid, path) }
	if strings.Contains(v.Relative, "\\") {
		return bad()
	}
	if v.Relative != "" {
		for _, part := range strings.Split(v.Relative, "/") {
			if part == "" || part == "." || part == ".." {
				return bad()
			}
		}
	}
	if target {
		if s.v.Runtime.Kind == ExecutionRuntimeHost {
			if v.Root != "workspace" {
				return bad()
			}
		} else if v.Root != "image" && !strings.HasPrefix(v.Root, "image:") {
			return bad()
		}
	}
	switch {
	case v.Root == "tool":
		if s.v.Runtime.Kind != ExecutionRuntimeHost {
			return bad()
		}
		if s.v.Runtime.IdentityKind == ExecutionIdentityHostExecutable && (v.Relative != "" || directory) {
			return bad()
		}
	case v.Root == "workspace":
		if s.v.Runtime.Kind != ExecutionRuntimeHost {
			return bad()
		}
	case v.Root == "image":
		if s.v.Runtime.Kind == ExecutionRuntimeHost || s.v.Runtime.Platform.OS == "windows" {
			return bad()
		}
	case strings.HasPrefix(v.Root, "image:"):
		if s.v.Runtime.Kind == ExecutionRuntimeHost || s.v.Runtime.Platform.OS != "windows" || len(v.Root) != 7 || v.Root[6] < 'a' || v.Root[6] > 'z' {
			return bad()
		}
	case strings.HasPrefix(v.Root, "mount:"):
		index, ok := s.mounts[strings.TrimPrefix(v.Root, "mount:")]
		if !ok {
			return bad()
		}
		mount := s.v.Mounts[index]
		if mount.Kind == ExecutionMountSecret {
			if v.Relative != "" || directory {
				return bad()
			}
		}
		if mount.Kind == ExecutionMountInput {
			if i, ok := s.inputs[mount.InputName]; ok && s.v.Inputs[i].Profile == ExecutionInputRegularFile && (v.Relative != "" || directory) {
				return bad()
			}
		}
	case strings.HasPrefix(v.Root, "service:"):
		parts := strings.Split(v.Root, ":")
		if len(parts) != 3 || v.Relative != "" {
			return bad()
		}
		index, ok := s.services[parts[1]]
		if !ok {
			return bad()
		}
		service := s.v.Services[index]
		switch parts[2] {
		case "socket":
			if service.Transport != ExecutionServiceUnixSocket || directory {
				return bad()
			}
		case "socket-directory":
			if service.Transport != ExecutionServiceUnixSocket {
				return bad()
			}
		case "credential-file":
			if service.AuthenticationSecret == "" || service.AuthenticationProfile == "" || directory {
				return bad()
			}
		default:
			return bad()
		}
		s.serviceUsed[parts[1]] = true
	default:
		return bad()
	}
	return nil
}

func (s *executionValidation) word(value ExecutionText, secrets bool, path string) (ExecutionText, error) {
	result := make(ExecutionText, 0, len(value))
	bad := func() (ExecutionText, error) { return nil, executionError(runtimev2.CodeValueInvalid, path) }
	for _, p := range value {
		switch p.Kind {
		case ExecutionTextPartLiteral:
			if p.SecretSlot != "" || p.ServiceRole != "" || p.ServiceField != "" || p.Path != nil || p.PathFormat != "" {
				return bad()
			}
			if p.Literal == "" {
				continue
			}
			if len(result) > 0 && result[len(result)-1].Kind == ExecutionTextPartLiteral {
				last := &result[len(result)-1]
				if len(p.Literal) > runtimev2.MaxCanonicalStringBytes-len(last.Literal) {
					return nil, executionError(runtimev2.CodeLimitExceeded, path)
				}
				last.Literal += p.Literal
				continue
			}
		case ExecutionTextPartSecret:
			if p.Literal != "" || p.ServiceRole != "" || p.ServiceField != "" || p.Path != nil || p.PathFormat != "" || !secrets {
				return bad()
			}
			index, ok := s.secrets[p.SecretSlot]
			if !ok || s.v.Secrets[index].Purpose != ExecutionSecretSemantic {
				return bad()
			}
			s.secretUsed[p.SecretSlot] = true
		case ExecutionTextPartService:
			if p.Literal != "" || p.SecretSlot != "" || p.Path != nil || p.PathFormat != "" {
				return bad()
			}
			index, ok := s.services[p.ServiceRole]
			if !ok {
				return bad()
			}
			service := s.v.Services[index]
			switch p.ServiceField {
			case ExecutionServiceHost, ExecutionServicePort:
				if service.Transport != ExecutionServiceTCP {
					return bad()
				}
			case ExecutionServiceDatabase:
				if service.Database == "" {
					return bad()
				}
			case ExecutionServiceUser:
				if service.User == "" {
					return bad()
				}
			default:
				return bad()
			}
			s.serviceUsed[p.ServiceRole] = true
		case ExecutionTextPartPath:
			if p.Literal != "" || p.SecretSlot != "" || p.ServiceRole != "" || p.ServiceField != "" || p.Path == nil || p.PathFormat != ExecutionPathNative && p.PathFormat != ExecutionPathForwardSlash {
				return bad()
			}
			if err := s.path(*p.Path, false, false, path); err != nil {
				return nil, err
			}
			copy := *p.Path
			p.Path = &copy
		default:
			return bad()
		}
		result = append(result, p)
	}
	if len(result) == 0 {
		result = ExecutionText{{Kind: ExecutionTextPartLiteral}}
	}
	return result, nil
}
