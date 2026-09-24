package resolver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

type snapshotStub struct {
	reader      *bytes.Reader
	statResults []os.FileInfo
	statErrors  []error
	statCalls   int
	seekErr     error
	readErr     error
	failPhase   int
	phase       int
}

func (s *snapshotStub) Read(p []byte) (int, error) {
	if s.readErr != nil && s.phase == s.failPhase {
		return 0, s.readErr
	}
	return s.reader.Read(p)
}
func (s *snapshotStub) Seek(offset int64, whence int) (int64, error) {
	if s.seekErr != nil {
		return 0, s.seekErr
	}
	s.phase++
	return s.reader.Seek(offset, whence)
}
func (s *snapshotStub) Stat() (os.FileInfo, error) {
	index := s.statCalls
	s.statCalls++
	if index < len(s.statErrors) && s.statErrors[index] != nil {
		return nil, s.statErrors[index]
	}
	if index < len(s.statResults) {
		return s.statResults[index], nil
	}
	return s.statResults[len(s.statResults)-1], nil
}

type changedFileInfo struct {
	os.FileInfo
	size     int64
	modified time.Time
}

type systemFileInfo struct {
	os.FileInfo
	system any
}

func (i systemFileInfo) Sys() any { return i.system }

type unsignedDevice struct{ Dev uint64 }
type signedDevice struct{ Dev int64 }
type invalidDevice struct{ Dev string }

func (i changedFileInfo) Size() int64        { return i.size }
func (i changedFileInfo) ModTime() time.Time { return i.modified }

func workspaceCoverageDeclaration(t *testing.T, owner, kind, schema string, fields []runtimev2.DeclarationField) runtimev2.InputDeclaration {
	t.Helper()
	value, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: owner, Kind: kind,
		SpecificationSchema: schema, Fields: fields,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestWorkspaceProviderNormalizationAndValidationBranches(t *testing.T) {
	providerValue, err := NewWorkspaceFileResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	provider := providerValue.(*workspaceFileResolver)
	descriptor := provider.Descriptor()
	if descriptor.Role != "input" || descriptor.Owner != workspaceOwner || descriptor.Kind != workspaceKind {
		t.Fatalf("descriptor: %+v", descriptor)
	}
	workspace := Workspace{Root: t.TempDir()}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	valid := workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "path", Value: "dir/file.sql"}})
	if _, err := provider.Normalize(cancelled, workspace, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}

	invalidDeclarations := []runtimev2.InputDeclaration{
		workspaceCoverageDeclaration(t, "other", workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "path", Value: "x"}}),
		workspaceCoverageDeclaration(t, workspaceOwner, "other", workspaceSpecification, []runtimev2.DeclarationField{{Name: "path", Value: "x"}}),
		workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, "other.v1", []runtimev2.DeclarationField{{Name: "path", Value: "x"}}),
		workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{}),
		workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "other", Value: "x"}}),
	}
	for index, declaration := range invalidDeclarations {
		if _, err := provider.Normalize(context.Background(), workspace, declaration); !errors.Is(err, ErrInvalidDeclaration) {
			t.Errorf("declaration %d: %v", index, err)
		}
	}
	for _, name := range []string{".", "..", "../x", "a/../../x", "a.", "a ", "COM1.txt", "dir//..", "a:b", "\\server\\share", string([]byte{'x', 0, 'y'})} {
		declaration := workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "path", Value: name}})
		if _, err := provider.Normalize(context.Background(), workspace, declaration); !errors.Is(err, ErrInvalidDeclaration) {
			t.Errorf("path %q: %v", name, err)
		}
	}
	normalized, err := provider.Normalize(context.Background(), workspace, valid)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Declaration.Fields()[0].Value != "dir/file.sql" {
		t.Fatal("normalization mismatch")
	}

	identity := func(owner, kind, schema string, fields []runtimev2.ResolvedField) runtimev2.ResolvedExtensionIdentity {
		value, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{SchemaVersion: runtimev2.SchemaVersion, Owner: owner, Kind: kind, IdentitySchema: schema, Fields: fields})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	digest := digestOf([]byte("x"))
	validIdentity := identity(workspaceOwner, workspaceKind, workspaceIdentity, []runtimev2.ResolvedField{{Name: "content.digest", Value: digest}})
	invalid := []Resolution{
		{Identity: identity("other", workspaceKind, workspaceIdentity, []runtimev2.ResolvedField{{Name: "content.digest", Value: digest}}), Evidence: []byte(`{}`)},
		{Identity: identity(workspaceOwner, workspaceKind, workspaceIdentity, []runtimev2.ResolvedField{}), Evidence: []byte(`{}`)},
		{Identity: identity(workspaceOwner, workspaceKind, workspaceIdentity, []runtimev2.ResolvedField{{Name: "other", Value: digest}}), Evidence: []byte(`{}`)},
		{Identity: validIdentity, Evidence: []byte(`{"schema_version":"bad","path":"x","size":1}`)},
		{Identity: validIdentity, Evidence: []byte(`{"schema_version":"sqlrs.workspace-file.evidence.v1","path":"../x","size":1}`)},
		{Identity: validIdentity, Evidence: []byte(`{"schema_version":"sqlrs.workspace-file.evidence.v1","path":"x","size":1,"extra":true}`)},
		{Identity: validIdentity, Evidence: []byte(`{"schema_version":"sqlrs.workspace-file.evidence.v1","path":"x","size":1} {}`)},
		{Identity: validIdentity, Evidence: []byte(`{"schema_version":"sqlrs.workspace-file.evidence.v1","path":"a/../x","size":1}`)},
		{Identity: validIdentity, Evidence: []byte(`{"schema_version":"sqlrs.workspace-file.evidence.v1","path":"x","size":-1}`)},
		{Identity: validIdentity, Evidence: bytes.Repeat([]byte("x"), maxProviderEvidenceBytes+1)},
		{Identity: validIdentity, Evidence: func() []byte {
			raw, _ := json.Marshal(fileEvidence{SchemaVersion: "sqlrs.workspace-file.evidence.v1", Path: "x", Size: 1, FilesystemClass: "ntfs-usn", EvidenceRevision: "ntfs-usn-v1", Strong: true})
			return raw
		}()},
	}
	for index, value := range invalid {
		if err := provider.ValidateResolution(value); !errors.Is(err, ErrInvalidDeclaration) {
			t.Errorf("resolution %d: %v", index, err)
		}
	}
}

func TestWorkspaceResolveRevalidateAcquireFailureBranches(t *testing.T) {
	providerValue, _ := NewWorkspaceFileResolver(nil)
	provider := providerValue.(*workspaceFileResolver)
	workspace := Workspace{Root: t.TempDir()}
	if _, err := provider.Resolve(context.Background(), workspace, NormalizedDeclaration{Declaration: coverageDeclaration(t)}); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("bad fields: %v", err)
	}
	wrongField := workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "other", Value: "missing"}})
	if _, err := provider.Resolve(context.Background(), workspace, NormalizedDeclaration{Declaration: wrongField}); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("wrong normalized field: %v", err)
	}
	noncanonical := workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "path", Value: "dir/../missing"}})
	if _, err := provider.Resolve(context.Background(), workspace, NormalizedDeclaration{Declaration: noncanonical}); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("noncanonical normalized path: %v", err)
	}

	declaration := workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "path", Value: "missing"}})
	if _, err := provider.Resolve(context.Background(), workspace, NormalizedDeclaration{Declaration: declaration}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root, "cancelled"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cancelDeclaration := workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "path", Value: "cancelled"}})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.Resolve(cancelled, workspace, NormalizedDeclaration{Declaration: cancelDeclaration}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel resolve: %v", err)
	}
	weakEvidence, _ := json.Marshal(fileEvidence{SchemaVersion: "sqlrs.workspace-file.evidence.v1", Path: "missing", Size: 1, FilesystemClass: "unsupported"})
	resolution := Resolution{Identity: func() runtimev2.ResolvedExtensionIdentity {
		v, _ := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{SchemaVersion: runtimev2.SchemaVersion, Owner: workspaceOwner, Kind: workspaceKind, IdentitySchema: workspaceIdentity, Fields: []runtimev2.ResolvedField{{Name: "content.digest", Value: digestOf([]byte("x"))}}})
		return v
	}(), Evidence: weakEvidence}
	status, err := provider.Revalidate(context.Background(), workspace, resolution)
	if err != nil || status.Status != Stale {
		t.Fatalf("missing revalidate: %+v %v", status, err)
	}
	if _, err := provider.Revalidate(cancelled, workspace, resolution); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel revalidate: %v", err)
	}
	if _, err := provider.Acquire(context.Background(), workspace, resolution); err == nil || !strings.Contains(err.Error(), "artifact store unavailable") {
		t.Fatalf("nil store: %v", err)
	}
	store, _ := NewDirectoryArtifactStore(t.TempDir())
	withStoreValue, _ := NewWorkspaceFileResolver(store)
	withStore := withStoreValue.(*workspaceFileResolver)
	if _, err := withStore.Acquire(context.Background(), workspace, Resolution{}); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("invalid acquire: %v", err)
	}
	if _, err := withStore.Acquire(context.Background(), workspace, resolution); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing acquire: %v", err)
	}
	if _, err := provider.Revalidate(context.Background(), workspace, Resolution{}); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("invalid revalidate: %v", err)
	}

	file := filepath.Join(workspace.Root, "input")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openSafeFile(filepath.Join(workspace.Root, "absent"), "x"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("root missing: %v", err)
	}
	if _, _, err := openSafeFile(workspace.Root, "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file missing: %v", err)
	}
	if _, _, err := openSafeFile(workspace.Root, "input/child"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("file component: %v", err)
	}
	directory := filepath.Join(workspace.Root, "directory")
	_ = os.Mkdir(directory, 0o700)
	if _, _, err := openSafeFile(workspace.Root, "directory/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing nested: %v", err)
	}
	if _, _, err := openSafeFile(workspace.Root, "directory"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory target: %v", err)
	}
	link := filepath.Join(workspace.Root, "linkdir")
	if err := os.Symlink(directory, link); err == nil {
		if _, _, err := openSafeFile(workspace.Root, "linkdir/file"); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("link component: %v", err)
		}
	}
	rootFile := filepath.Join(t.TempDir(), "root-file")
	if err := os.WriteFile(rootFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Revalidate(context.Background(), Workspace{Root: rootFile}, resolution); err == nil {
		t.Fatal("file workspace root accepted")
	}
}

func TestOpenSafeFileRejectsReplacementBetweenWalkAndOpen(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.sql")
	replacement := filepath.Join(root, "replacement.sql")
	retired := filepath.Join(root, "retired.sql")
	if err := os.WriteFile(target, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := openSafeFileAfterWalk(root, "target.sql", func() {
		if renameErr := os.Rename(target, retired); renameErr != nil {
			t.Fatalf("retire target: %v", renameErr)
		}
		if renameErr := os.Rename(replacement, target); renameErr != nil {
			t.Fatalf("publish replacement: %v", renameErr)
		}
	})
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("replacement error = %v", err)
	}
}

func TestWorkspaceResolveDetectsConcurrentGrowth(t *testing.T) {
	workspace := Workspace{Root: t.TempDir()}
	name := filepath.Join(workspace.Root, "growing")
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(64 << 20); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	declaration := workspaceCoverageDeclaration(t, workspaceOwner, workspaceKind, workspaceSpecification, []runtimev2.DeclarationField{{Name: "path", Value: "growing"}})
	providerValue, _ := NewWorkspaceFileResolver(nil)
	provider := providerValue.(*workspaceFileResolver)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		writer, openErr := os.OpenFile(name, os.O_WRONLY|os.O_APPEND, 0)
		if openErr != nil {
			return
		}
		defer writer.Close()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = writer.Write([]byte{1})
				time.Sleep(time.Millisecond)
			}
		}
	}()
	_, resolveErr := provider.Resolve(context.Background(), workspace, NormalizedDeclaration{Declaration: declaration})
	close(stop)
	<-done
	if !errors.Is(resolveErr, ErrChanged) {
		t.Fatalf("concurrent growth = %v", resolveErr)
	}
}

func TestStableFileDigestFailureMatrix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := changedFileInfo{FileInfo: info, size: info.Size() + 1, modified: info.ModTime()}
	boom := errors.New("boom")
	tests := []struct {
		name string
		stub *snapshotStub
		want error
	}{
		{"first read", &snapshotStub{reader: bytes.NewReader([]byte("abc")), readErr: boom, failPhase: 0, statResults: []os.FileInfo{info}}, boom},
		{"first stat", &snapshotStub{reader: bytes.NewReader([]byte("abc")), statResults: []os.FileInfo{info}, statErrors: []error{boom}}, boom},
		{"first changed", &snapshotStub{reader: bytes.NewReader([]byte("abc")), statResults: []os.FileInfo{changed}}, ErrChanged},
		{"seek", &snapshotStub{reader: bytes.NewReader([]byte("abc")), statResults: []os.FileInfo{info}, seekErr: boom}, boom},
		{"second read", &snapshotStub{reader: bytes.NewReader([]byte("abc")), statResults: []os.FileInfo{info}, readErr: boom, failPhase: 1}, boom},
		{"second stat", &snapshotStub{reader: bytes.NewReader([]byte("abc")), statResults: []os.FileInfo{info, info}, statErrors: []error{nil, boom}}, boom},
		{"second changed", &snapshotStub{reader: bytes.NewReader([]byte("abc")), statResults: []os.FileInfo{info, changed}}, ErrChanged},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := stableFileDigest(context.Background(), test.stub, info); !errors.Is(err, test.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	good := &snapshotStub{reader: bytes.NewReader([]byte("abc")), statResults: []os.FileInfo{info, info}}
	if digest, err := stableFileDigest(context.Background(), good, info); err != nil || digest != digestOf([]byte("abc")) {
		t.Fatalf("digest=%s err=%v", digest, err)
	}
}

func TestFilesystemTransitionClassification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if filesystemTransition(systemFileInfo{info, unsignedDevice{1}}, systemFileInfo{info, unsignedDevice{1}}) {
		t.Fatal("same device changed")
	}
	if !filesystemTransition(systemFileInfo{info, unsignedDevice{1}}, systemFileInfo{info, unsignedDevice{2}}) {
		t.Fatal("transition missed")
	}
	if !filesystemTransition(systemFileInfo{info, signedDevice{1}}, systemFileInfo{info, signedDevice{2}}) {
		t.Fatal("signed transition missed")
	}
	for _, system := range []any{nil, struct{}{}, invalidDevice{"x"}, "not-struct"} {
		if filesystemTransition(systemFileInfo{info, system}, systemFileInfo{info, unsignedDevice{2}}) {
			t.Fatalf("unsupported system accepted: %T", system)
		}
	}
}
