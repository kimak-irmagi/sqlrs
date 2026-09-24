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
	SchemaVersion    string `json:"schema_version"`
	Path             string `json:"path"`
	Size             int64  `json:"size"`
	ModifiedNanos    int64  `json:"modified_nanos"`
	FilesystemClass  string `json:"filesystem_class"`
	EvidenceRevision string `json:"evidence_revision"`
	Strong           bool   `json:"strong"`
	VolumeID         string `json:"volume_id"`
	FileID           string `json:"file_id"`
	ChangeToken      string `json:"change_token"`
}
type continuityEvidence struct{ Class, Revision, VolumeID, FileID, ChangeToken string }
type workspaceFileResolver struct{ artifacts ArtifactStore }

// NewWorkspaceFileResolver constructs the reference rooted-file provider.
func NewWorkspaceFileResolver(artifacts ArtifactStore) (Resolver, error) {
	if nilInterface(artifacts) {
		artifacts = nil
	}
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
	if normalized.Declaration.Role() != "input" || normalized.Declaration.Owner() != workspaceOwner || normalized.Declaration.Kind() != workspaceKind || normalized.Declaration.SpecificationSchema() != workspaceSpecification {
		return Resolution{}, ErrInvalidDeclaration
	}
	fields := normalized.Declaration.Fields()
	if len(fields) != 1 || fields[0].Name != "path" {
		return Resolution{}, ErrInvalidDeclaration
	}
	cleaned, err := normalizeWorkspacePath(fields[0].Value)
	if err != nil || cleaned != fields[0].Value {
		return Resolution{}, ErrInvalidDeclaration
	}
	file, info, err := openSafeFile(workspace.Root, fields[0].Value)
	if err != nil {
		return Resolution{}, err
	}
	defer file.Close()
	digest, err := stableFileDigest(ctx, file, info)
	if err != nil {
		return Resolution{}, err
	}
	// Every constructor input below is a package constant except digest, which
	// stableFileDigest emits in the constructor's required lowercase format.
	identity, _ := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{SchemaVersion: runtimev2.SchemaVersion, Owner: workspaceOwner, Kind: workspaceKind, IdentitySchema: workspaceIdentity, Fields: []runtimev2.ResolvedField{{Name: "content.digest", Value: digest}}})
	native := nativeContinuityEvidence(file, info)
	strong := cheapRevalidationEnabled(native.Class, native.Revision) && native.VolumeID != "" && native.FileID != "" && native.ChangeToken != ""
	evidence, _ := json.Marshal(fileEvidence{SchemaVersion: "sqlrs.workspace-file.evidence.v1", Path: fields[0].Value, Size: info.Size(), ModifiedNanos: info.ModTime().UnixNano(), FilesystemClass: native.Class, EvidenceRevision: native.Revision, Strong: strong, VolumeID: native.VolumeID, FileID: native.FileID, ChangeToken: native.ChangeToken})
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
	if len(value.Evidence) == 0 || len(value.Evidence) > maxProviderEvidenceBytes {
		return ErrInvalidDeclaration
	}
	if rejectDuplicateJSON(value.Evidence) != nil {
		return ErrInvalidDeclaration
	}
	decoder := json.NewDecoder(strings.NewReader(string(value.Evidence)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&evidence) != nil || evidence.SchemaVersion != "sqlrs.workspace-file.evidence.v1" {
		return ErrInvalidDeclaration
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalidDeclaration
	}
	cleaned, err := normalizeWorkspacePath(evidence.Path)
	if err != nil || cleaned != evidence.Path || evidence.Size < 0 || evidence.FilesystemClass == "" {
		return ErrInvalidDeclaration
	}
	if evidence.Strong && (!cheapRevalidationEnabled(evidence.FilesystemClass, evidence.EvidenceRevision) || evidence.VolumeID == "" || evidence.FileID == "" || evidence.ChangeToken == "") {
		return ErrInvalidDeclaration
	}
	return nil
}

const maxProviderEvidenceBytes = 64 << 10

type snapshotFile interface {
	io.Reader
	io.Seeker
	Stat() (os.FileInfo, error)
}

// stableFileDigest rejects ordinary in-place mutation by requiring two equal
// reads and unchanged metadata from the same safely opened file descriptor.
func stableFileDigest(ctx context.Context, file snapshotFile, initial os.FileInfo) (string, error) {
	digest, err := hashReader(ctx, file)
	if err != nil {
		return "", err
	}
	after, err := file.Stat()
	if err != nil {
		return "", err
	}
	if after.Size() != initial.Size() || !after.ModTime().Equal(initial.ModTime()) {
		return "", ErrChanged
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	confirmation, err := hashReader(ctx, file)
	if err != nil {
		return "", err
	}
	confirmed, err := file.Stat()
	if err != nil {
		return "", err
	}
	if confirmation != digest || confirmed.Size() != initial.Size() || !confirmed.ModTime().Equal(initial.ModTime()) {
		return "", ErrChanged
	}
	return digest, nil
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
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrUnsafePath) || errors.Is(err, ErrNotRegular) {
			return Revalidation{Status: Stale, Reason: "source_missing_or_replaced"}, nil
		}
		return Revalidation{}, err
	}
	current := nativeContinuityEvidence(file, info)
	file.Close()
	if info.Size() != evidence.Size {
		return Revalidation{Status: Stale, Reason: "size_changed"}, nil
	}
	if evidence.Strong && cheapRevalidationEnabled(current.Class, current.Revision) && current.VolumeID == evidence.VolumeID && current.FileID == evidence.FileID && current.ChangeToken == evidence.ChangeToken && info.ModTime().UnixNano() == evidence.ModifiedNanos {
		return Revalidation{Status: Current, Reason: "strong_filesystem_continuity", Evidence: append(json.RawMessage(nil), value.Evidence...)}, nil
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
	return openSafeFileAfterWalk(rootPath, name, nil)
}

// openSafeFileAfterWalk exposes the security-sensitive walk/open boundary to
// deterministic package tests without changing the public resolver contract.
func openSafeFileAfterWalk(rootPath, name string, afterWalk func()) (*os.File, os.FileInfo, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return nil, nil, err
	}
	current := ""
	parts := strings.Split(name, "/")
	observed := make([]os.FileInfo, len(parts))
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
		if isLinkLike(info) {
			return nil, nil, ErrUnsafePath
		}
		if filesystemTransition(rootInfo, info) {
			return nil, nil, ErrUnsafePath
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, nil, ErrUnsafePath
		}
		observed[index] = info
	}
	if afterWalk != nil {
		afterWalk()
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
		return nil, nil, ErrNotRegular
	}
	if !os.SameFile(observed[len(observed)-1], info) {
		file.Close()
		return nil, nil, ErrUnsafePath
	}
	// Recheck every component after opening. os.Root confines traversal to the
	// root; matching file identities additionally rejects a stable rename/link
	// substitution that occurred between the first walk and the open.
	for index, component := range parts {
		if index == 0 {
			current = component
		} else {
			current += "/" + component
		}
		after, afterErr := root.Lstat(current)
		if afterErr != nil || isLinkLike(after) || filesystemTransition(rootInfo, after) || !os.SameFile(observed[index], after) {
			file.Close()
			return nil, nil, ErrUnsafePath
		}
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
