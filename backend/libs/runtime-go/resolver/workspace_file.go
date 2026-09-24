package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

const (
	workspaceOwner         = "sqlrs.workspace"
	workspaceKind          = "file"
	workspaceSpecification = "sqlrs.workspace-file.declaration.v1"
	workspaceIdentity      = "sqlrs.workspace-file.v1"
)

type fileEvidence struct {
	SchemaVersion string `json:"schema_version"`
	Path          string `json:"path"`
	Size          int64  `json:"size"`
}
type workspaceFileResolver struct{ artifacts ArtifactStore }

// NewWorkspaceFileResolver constructs the reference rooted-file provider.
func NewWorkspaceFileResolver(artifacts ArtifactStore) (Resolver, error) {
	return &workspaceFileResolver{artifacts: artifacts}, nil
}
func (r *workspaceFileResolver) Descriptor() Descriptor {
	return Descriptor{Role: "input", Owner: workspaceOwner, Kind: workspaceKind, SpecificationSchema: workspaceSpecification, SemanticVersion: "1"}
}
func (r *workspaceFileResolver) Normalize(ctx context.Context, workspace Workspace, declaration runtimev2.InputDeclaration) (NormalizedDeclaration, error) {
	if err := ctx.Err(); err != nil {
		return NormalizedDeclaration{}, err
	}
	if declaration.Role() != "input" || declaration.Owner() != workspaceOwner || declaration.Kind() != workspaceKind || declaration.SpecificationSchema() != workspaceSpecification {
		return NormalizedDeclaration{}, ErrInvalidDeclaration
	}
	fields := declaration.Fields()
	if len(fields) != 1 || fields[0].Name != "path" {
		return NormalizedDeclaration{}, ErrInvalidDeclaration
	}
	cleaned, err := normalizeWorkspacePath(fields[0].Value)
	if err != nil {
		return NormalizedDeclaration{}, err
	}
	value, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: workspaceOwner, Kind: workspaceKind, SpecificationSchema: workspaceSpecification, Fields: []runtimev2.DeclarationField{{Name: "path", Value: cleaned}}})
	if err != nil {
		return NormalizedDeclaration{}, fmt.Errorf("%w: %v", ErrInvalidDeclaration, err)
	}
	return NormalizedDeclaration{Declaration: value}, nil
}
func normalizeWorkspacePath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, 0) {
		return "", ErrInvalidDeclaration
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if strings.HasPrefix(value, "/") || filepath.IsAbs(value) || filepath.VolumeName(value) != "" || strings.Contains(value, ":") {
		return "", ErrInvalidDeclaration
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrInvalidDeclaration
	}
	reserved := map[string]bool{"con": true, "prn": true, "aux": true, "nul": true, "com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true, "lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true}
	for _, component := range strings.Split(cleaned, "/") {
		if component == "" || strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
			return "", ErrInvalidDeclaration
		}
		base := strings.ToLower(strings.Split(component, ".")[0])
		if reserved[base] {
			return "", ErrInvalidDeclaration
		}
	}
	return cleaned, nil
}
func (r *workspaceFileResolver) Resolve(ctx context.Context, workspace Workspace, normalized NormalizedDeclaration) (Resolution, error) {
	fields := normalized.Declaration.Fields()
	if len(fields) != 1 {
		return Resolution{}, ErrInvalidDeclaration
	}
	file, info, err := openSafeFile(workspace.Root, fields[0].Value)
	if err != nil {
		return Resolution{}, err
	}
	defer file.Close()
	digest, err := hashReader(ctx, file)
	if err != nil {
		return Resolution{}, err
	}
	after, err := file.Stat()
	if err != nil {
		return Resolution{}, err
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return Resolution{}, ErrChanged
	}
	identity, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{SchemaVersion: runtimev2.SchemaVersion, Owner: workspaceOwner, Kind: workspaceKind, IdentitySchema: workspaceIdentity, Fields: []runtimev2.ResolvedField{{Name: "content.digest", Value: digest}}})
	if err != nil {
		return Resolution{}, err
	}
	evidence, _ := json.Marshal(fileEvidence{SchemaVersion: "sqlrs.workspace-file.evidence.v1", Path: fields[0].Value, Size: info.Size()})
	return Resolution{Identity: identity, Evidence: evidence}, nil
}
func (r *workspaceFileResolver) ValidateResolution(value Resolution) error {
	if value.Identity.Owner() != workspaceOwner || value.Identity.Kind() != workspaceKind || value.Identity.IdentitySchema() != workspaceIdentity {
		return ErrInvalidDeclaration
	}
	fields := value.Identity.Fields()
	if len(fields) != 1 || fields[0].Name != "content.digest" || !validDigest(fields[0].Value) {
		return ErrInvalidDeclaration
	}
	var evidence fileEvidence
	decoder := json.NewDecoder(strings.NewReader(string(value.Evidence)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&evidence) != nil || evidence.SchemaVersion != "sqlrs.workspace-file.evidence.v1" {
		return ErrInvalidDeclaration
	}
	if _, err := normalizeWorkspacePath(evidence.Path); err != nil {
		return ErrInvalidDeclaration
	}
	return nil
}
func (r *workspaceFileResolver) Revalidate(ctx context.Context, workspace Workspace, value Resolution) (Revalidation, error) {
	if err := ctx.Err(); err != nil {
		return Revalidation{}, err
	}
	if err := r.ValidateResolution(value); err != nil {
		return Revalidation{}, err
	}
	var evidence fileEvidence
	_ = json.Unmarshal(value.Evidence, &evidence)
	file, info, err := openSafeFile(workspace.Root, evidence.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrUnsafePath) {
			return Revalidation{Status: Stale, Reason: "source_missing_or_replaced"}, nil
		}
		return Revalidation{}, err
	}
	file.Close()
	if info.Size() != evidence.Size {
		return Revalidation{Status: Stale, Reason: "size_changed"}, nil
	}
	return Revalidation{Status: Unknown, Reason: "filesystem_evidence_disabled", Evidence: append(json.RawMessage(nil), value.Evidence...)}, nil
}
func (r *workspaceFileResolver) Acquire(ctx context.Context, workspace Workspace, value Resolution) (Artifact, error) {
	if r.artifacts == nil {
		return nil, errors.New("resolver: artifact store unavailable")
	}
	if err := r.ValidateResolution(value); err != nil {
		return nil, err
	}
	var evidence fileEvidence
	_ = json.Unmarshal(value.Evidence, &evidence)
	file, _, err := openSafeFile(workspace.Root, evidence.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return r.artifacts.PublishVerified(ctx, file, value.Identity.Fields()[0].Value)
}
func openSafeFile(rootPath, name string) (*os.File, os.FileInfo, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	current := ""
	parts := strings.Split(name, "/")
	for index, part := range parts {
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		info, err := root.Lstat(current)
		if err != nil {
			return nil, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, nil, ErrUnsafePath
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, nil, ErrUnsafePath
		}
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, ErrUnsafePath
	}
	return file, info, nil
}
func hashReader(ctx context.Context, reader io.Reader) (string, error) {
	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		count, err := reader.Read(buffer)
		if count > 0 {
			_, _ = hash.Write(buffer[:count])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
func validDigest(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	raw, err := hex.DecodeString(value[7:])
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value[7:]
}
