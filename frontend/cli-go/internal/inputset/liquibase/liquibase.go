// Package liquibase owns the shared Liquibase file-bearing semantics described in
// docs/architecture/inputset-component-structure.md.
package liquibase

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sqlrs/cli/internal/inputset"
	"gopkg.in/yaml.v3"
)

type boolish string

func (b *boolish) UnmarshalYAML(node *yaml.Node) error {
	*b = boolish(node.Value)
	return nil
}

func (b *boolish) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("true")) || bytes.Equal(data, []byte("false")) {
		*b = boolish(string(data))
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*b = boolish(s)
	return nil
}

func (b boolish) useChangelogDir() bool {
	return !strings.EqualFold(strings.TrimSpace(string(b)), "false")
}

type xmlChangeLog struct {
	XMLName    xml.Name       `xml:"databaseChangeLog"`
	Includes   []includeSpec  `xml:"include"`
	IncludeAll []includeAll   `xml:"includeAll"`
	ChangeSets []xmlChangeSet `xml:"changeSet"`
}

type structuredChangeLog struct {
	DatabaseChangeLog []changeItem `yaml:"databaseChangeLog" json:"databaseChangeLog"`
}

type changeItem struct {
	Include    *includeSpec   `yaml:"include" json:"include"`
	IncludeAll *includeAll    `yaml:"includeAll" json:"includeAll"`
	ChangeSet  *changeSetSpec `yaml:"changeSet" json:"changeSet"`
}

type includeSpec struct {
	File                    string  `xml:"file,attr" yaml:"file" json:"file"`
	RelativeToChangelogFile boolish `xml:"relativeToChangelogFile,attr" yaml:"relativeToChangelogFile" json:"relativeToChangelogFile"`
}

type includeAll struct {
	Path                    string  `xml:"path,attr" yaml:"path" json:"path"`
	RelativeToChangelogFile boolish `xml:"relativeToChangelogFile,attr" yaml:"relativeToChangelogFile" json:"relativeToChangelogFile"`
}

type xmlChangeSet struct {
	SQLFiles       []localPathRef `xml:"sqlFile"`
	LoadData       []localPathRef `xml:"loadData"`
	LoadUpdateData []localPathRef `xml:"loadUpdateData"`
}

type changeSetSpec struct {
	SQLFile        *localPathRef     `yaml:"sqlFile" json:"sqlFile"`
	LoadData       *localPathRef     `yaml:"loadData" json:"loadData"`
	LoadUpdateData *localPathRef     `yaml:"loadUpdateData" json:"loadUpdateData"`
	Changes        []changeSetChange `yaml:"changes" json:"changes"`
}

type changeSetChange struct {
	SQLFile        *localPathRef `yaml:"sqlFile" json:"sqlFile"`
	LoadData       *localPathRef `yaml:"loadData" json:"loadData"`
	LoadUpdateData *localPathRef `yaml:"loadUpdateData" json:"loadUpdateData"`
}

type localPathRef struct {
	File                    string  `xml:"file,attr" yaml:"file" json:"file"`
	Path                    string  `xml:"path,attr" yaml:"path" json:"path"`
	RelativeToChangelogFile boolish `xml:"relativeToChangelogFile,attr" yaml:"relativeToChangelogFile" json:"relativeToChangelogFile"`
}

func (r localPathRef) rawPath() string {
	if strings.TrimSpace(r.Path) != "" {
		return r.Path
	}
	return r.File
}

type declaredPath struct {
	flag        string
	path        string
	requireFile bool
}

// NormalizeArgs applies shared Liquibase path-bearing normalization.
func NormalizeArgs(args []string, resolver inputset.Resolver, canonicalSearchPath bool) ([]string, error) {
	normalized := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file" || arg == "--searchPath" || arg == "--search-path":
			if i+1 >= len(args) {
				return nil, inputset.Errorf("missing_path_arg", "Missing value for %s", arg)
			}
			flag := normalizeFlag(arg, canonicalSearchPath)
			value, err := rewriteValue(flag, args[i+1], resolver)
			if err != nil {
				return nil, err
			}
			normalized = append(normalized, flag, value)
			i++
		case strings.HasPrefix(arg, "--changelog-file="):
			value, err := rewriteValue("--changelog-file", strings.TrimPrefix(arg, "--changelog-file="), resolver)
			if err != nil {
				return nil, err
			}
			normalized = append(normalized, "--changelog-file="+value)
		case strings.HasPrefix(arg, "--defaults-file="):
			value, err := rewriteValue("--defaults-file", strings.TrimPrefix(arg, "--defaults-file="), resolver)
			if err != nil {
				return nil, err
			}
			normalized = append(normalized, "--defaults-file="+value)
		case strings.HasPrefix(arg, "--searchPath="):
			value, err := rewriteValue("--searchPath", strings.TrimPrefix(arg, "--searchPath="), resolver)
			if err != nil {
				return nil, err
			}
			normalized = append(normalized, "--searchPath="+value)
		case strings.HasPrefix(arg, "--search-path="):
			value, err := rewriteValue("--searchPath", strings.TrimPrefix(arg, "--search-path="), resolver)
			if err != nil {
				return nil, err
			}
			if canonicalSearchPath {
				normalized = append(normalized, "--searchPath="+value)
			} else {
				normalized = append(normalized, "--search-path="+value)
			}
		default:
			normalized = append(normalized, arg)
		}
	}

	return normalized, nil
}

// Collect builds the deterministic Liquibase direct-input and changelog-graph set.
func Collect(args []string, resolver inputset.Resolver, fs inputset.FileSystem) (inputset.InputSet, error) {
	declared, searchPaths, changelogPath, err := collectDeclared(args, resolver)
	if err != nil {
		return inputset.InputSet{}, err
	}
	if changelogPath == "" {
		return inputset.InputSet{}, fmt.Errorf("liquibase command has no --changelog-file (required for diff)")
	}
	return collectInputSet(declared, searchPaths, resolver, fs, nil)
}

// CollectInvocationInputs builds the deterministic Liquibase input set for a
// concrete invocation and includes the changelog graph when a changelog file is
// declared.
func CollectInvocationInputs(args []string, resolver inputset.Resolver, fs inputset.FileSystem) (inputset.InputSet, error) {
	declared, searchPaths, changelogPath, err := collectDeclared(args, resolver)
	if err != nil {
		return inputset.InputSet{}, err
	}
	var snapshots map[string]string
	declared, searchPaths, snapshots, err = includeDefaultsPaths(declared, searchPaths, changelogPath, resolver, fs)
	if err != nil {
		return inputset.InputSet{}, err
	}
	return collectInputSet(declared, searchPaths, resolver, fs, snapshots)
}

// includeDefaultsPaths adds the explicit or automatically discovered settings
// file and its file-bearing properties to the invocation closure. The
// supported properties are documented in
// docs/architecture/liquibase-container-component-structure.md.
func includeDefaultsPaths(declared []declaredPath, searchPaths []string, changelogPath string, resolver inputset.Resolver, fs inputset.FileSystem) ([]declaredPath, []string, map[string]string, error) {
	defaultsPath := ""
	for _, path := range declared {
		if path.flag == "--defaults-file" {
			defaultsPath = path.path
		}
	}
	if defaultsPath == "" {
		candidate := filepath.Join(resolver.BaseDir, "liquibase.properties")
		if info, err := fs.Stat(candidate); err == nil {
			if !info.Mode().IsRegular() {
				return nil, nil, nil, inputset.Errorf("unsupported_resource", "automatic Liquibase settings must be a regular file: %s", candidate)
			}
			defaultsPath = candidate
			declared = append([]declaredPath{{flag: "--defaults-file", path: candidate, requireFile: true}}, declared...)
		}
	}
	if defaultsPath == "" {
		return declared, searchPaths, nil, nil
	}
	content, err := fs.ReadFile(defaultsPath)
	if err != nil {
		return nil, nil, nil, err
	}
	properties, err := parseDefaultsFilePaths(content)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(searchPaths) == 0 && properties.searchPath != "" {
		searchPaths, err = resolveSearchPathParts(properties.searchPath, resolver)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if changelogPath == "" && properties.changelogFile != "" {
		if err := validateResourceRef(properties.changelogFile); err != nil {
			return nil, nil, nil, err
		}
		resolved := ""
		if !filepath.IsAbs(properties.changelogFile) {
			for _, base := range searchPaths {
				candidate := filepath.Clean(filepath.Join(base, filepath.FromSlash(properties.changelogFile)))
				if _, err := fs.Stat(candidate); err == nil {
					resolved = candidate
					break
				}
			}
		}
		if resolved == "" {
			resolved, err = resolver.ResolvePath(properties.changelogFile)
			if err != nil {
				return nil, nil, nil, err
			}
		}
		declared = append(declared, declaredPath{flag: "--changelog-file", path: resolved, requireFile: true})
	}
	return declared, searchPaths, map[string]string{defaultsPath: inputset.HashContent(content)}, nil
}

type defaultsFilePaths struct {
	changelogFile string
	searchPath    string
}

func parseDefaultsFilePaths(content []byte) (defaultsFilePaths, error) {
	var paths defaultsFilePaths
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if strings.Contains(line, "\\") {
			return defaultsFilePaths{}, inputset.Errorf("unsupported_resource", "escaped Liquibase settings are not yet supported")
		}
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			return defaultsFilePaths{}, inputset.Errorf("unsupported_resource", "unsupported Liquibase settings line: %s", line)
		}
		key := strings.TrimSpace(line[:sep])
		value := strings.TrimSpace(line[sep+1:])
		switch key {
		case "liquibase.changelogFile", "changelogFile":
			paths.changelogFile = value
		case "liquibase.searchPath", "searchPath":
			paths.searchPath = value
		}
	}
	return paths, nil
}

func collectInputSet(declared []declaredPath, searchPaths []string, resolver inputset.Resolver, fs inputset.FileSystem, snapshots map[string]string) (inputset.InputSet, error) {
	if snapshots == nil {
		snapshots = make(map[string]string)
	}
	tracker := &tracker{
		fs:          fs,
		root:        resolver.Root,
		searchPaths: searchPaths,
		seen:        make(map[string]struct{}),
		snapshots:   snapshots,
	}
	var order []string
	for _, ref := range declared {
		if ref.flag == "--changelog-file" {
			if err := tracker.collect(ref.path, &order); err != nil {
				return inputset.InputSet{}, err
			}
			continue
		}
		if err := tracker.addLeaf(ref.path, &order); err != nil {
			return inputset.InputSet{}, err
		}
	}

	return buildInputSetWithSnapshots(order, resolver.Root, fs, tracker.snapshots)
}

func buildInputSet(order []string, root string, fs inputset.FileSystem) (inputset.InputSet, error) {
	return buildInputSetWithSnapshots(order, root, fs, nil)
}

func buildInputSetWithSnapshots(order []string, root string, fs inputset.FileSystem, snapshots map[string]string) (inputset.InputSet, error) {
	entries := make([]inputset.InputEntry, 0, len(order))
	for _, path := range order {
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		content, err := fs.ReadFile(path)
		if err != nil {
			return inputset.InputSet{}, fmt.Errorf("read %s: %w", path, err)
		}
		contentHash := inputset.HashContent(content)
		if expected, ok := snapshots[path]; ok && contentHash != expected {
			return inputset.InputSet{}, inputset.Errorf("input_changed", "Liquibase input changed during capture: %s", rel)
		}
		var hash string
		if oid, ok := fs.(inputset.BlobOIDer); ok {
			h, err := oid.BlobOID(path)
			if err != nil {
				return inputset.InputSet{}, fmt.Errorf("hash %s: %w", path, err)
			}
			hash = h
		} else {
			hash = contentHash
		}
		entries = append(entries, inputset.InputEntry{
			Path:    rel,
			AbsPath: path,
			Hash:    hash,
		})
	}
	return inputset.InputSet{Entries: entries}, nil
}

// ValidateArgs accumulates alias-check issues for the shared Liquibase path syntax.
func ValidateArgs(args []string, resolver inputset.Resolver, fs inputset.FileSystem) []inputset.UserError {
	issues := make([]inputset.UserError, 0, 2)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file":
			if i+1 >= len(args) {
				issues = append(issues, *inputset.Errorf("missing_path_arg", "missing value for %s", arg))
				continue
			}
			if issue, ok := validateLocalArg(args[i+1], resolver, fs, true); ok {
				issues = append(issues, issue)
			}
			i++
		case arg == "--searchPath" || arg == "--search-path":
			if i+1 >= len(args) {
				issues = append(issues, *inputset.Errorf("missing_path_arg", "missing value for %s", arg))
				continue
			}
			issues = append(issues, validateSearchPath(args[i+1], resolver, fs)...)
			i++
		case strings.HasPrefix(arg, "--changelog-file="):
			if issue, ok := validateLocalArg(strings.TrimPrefix(arg, "--changelog-file="), resolver, fs, true); ok {
				issues = append(issues, issue)
			}
		case strings.HasPrefix(arg, "--defaults-file="):
			if issue, ok := validateLocalArg(strings.TrimPrefix(arg, "--defaults-file="), resolver, fs, true); ok {
				issues = append(issues, issue)
			}
		case strings.HasPrefix(arg, "--searchPath="):
			issues = append(issues, validateSearchPath(strings.TrimPrefix(arg, "--searchPath="), resolver, fs)...)
		case strings.HasPrefix(arg, "--search-path="):
			issues = append(issues, validateSearchPath(strings.TrimPrefix(arg, "--search-path="), resolver, fs)...)
		}
	}
	return issues
}

func normalizeFlag(flag string, canonicalSearchPath bool) string {
	if canonicalSearchPath && flag == "--search-path" {
		return "--searchPath"
	}
	return flag
}

func rewriteValue(flag string, value string, resolver inputset.Resolver) (string, error) {
	if flag == "--searchPath" || flag == "--search-path" {
		return rewriteSearchPath(value, resolver)
	}
	if strings.TrimSpace(value) == "" {
		return "", inputset.Errorf("empty_path", "Path is empty")
	}
	if inputset.LooksLikeLiquibaseRemoteRef(value) {
		return value, nil
	}
	return resolver.ResolvePath(value)
}

func rewriteSearchPath(value string, resolver inputset.Resolver) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", inputset.Errorf("empty_search_path", "searchPath is empty")
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			return "", inputset.Errorf("empty_search_path", "searchPath is empty")
		}
		if inputset.LooksLikeLiquibaseRemoteRef(item) {
			out = append(out, item)
			continue
		}
		path, err := resolver.ResolvePath(item)
		if err != nil {
			return "", err
		}
		out = append(out, path)
	}
	return strings.Join(out, ","), nil
}

func collectDeclared(args []string, resolver inputset.Resolver) ([]declaredPath, []string, string, error) {
	declared := make([]declaredPath, 0, 2)
	searchPaths := make([]string, 0, 2)
	changelogPath := ""

	appendSearchPaths := func(value string) error {
		parts, err := resolveSearchPathParts(value, resolver)
		if err != nil {
			return err
		}
		searchPaths = append(searchPaths, parts...)
		return nil
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file":
			if i+1 >= len(args) {
				return nil, nil, "", inputset.Errorf("missing_path_arg", "Missing value for %s", arg)
			}
			value, err := rewriteValue(arg, args[i+1], resolver)
			if err != nil {
				return nil, nil, "", err
			}
			if err := validateResourceRef(value); err != nil {
				return nil, nil, "", err
			}
			declared = append(declared, declaredPath{flag: arg, path: value, requireFile: true})
			if arg == "--changelog-file" {
				changelogPath = value
			}
			i++
		case arg == "--searchPath" || arg == "--search-path":
			if i+1 >= len(args) {
				return nil, nil, "", inputset.Errorf("missing_path_arg", "Missing value for %s", arg)
			}
			if err := appendSearchPaths(args[i+1]); err != nil {
				return nil, nil, "", err
			}
			i++
		case strings.HasPrefix(arg, "--changelog-file="):
			value, err := rewriteValue("--changelog-file", strings.TrimPrefix(arg, "--changelog-file="), resolver)
			if err != nil {
				return nil, nil, "", err
			}
			if err := validateResourceRef(value); err != nil {
				return nil, nil, "", err
			}
			declared = append(declared, declaredPath{flag: "--changelog-file", path: value, requireFile: true})
			changelogPath = value
		case strings.HasPrefix(arg, "--defaults-file="):
			value, err := rewriteValue("--defaults-file", strings.TrimPrefix(arg, "--defaults-file="), resolver)
			if err != nil {
				return nil, nil, "", err
			}
			if err := validateResourceRef(value); err != nil {
				return nil, nil, "", err
			}
			declared = append(declared, declaredPath{flag: "--defaults-file", path: value, requireFile: true})
		case strings.HasPrefix(arg, "--searchPath="):
			if err := appendSearchPaths(strings.TrimPrefix(arg, "--searchPath=")); err != nil {
				return nil, nil, "", err
			}
		case strings.HasPrefix(arg, "--search-path="):
			if err := appendSearchPaths(strings.TrimPrefix(arg, "--search-path=")); err != nil {
				return nil, nil, "", err
			}
		}
	}
	return declared, searchPaths, changelogPath, nil
}

func resolveSearchPathParts(value string, resolver inputset.Resolver) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, inputset.Errorf("empty_search_path", "searchPath is empty")
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			return nil, inputset.Errorf("empty_search_path", "searchPath is empty")
		}
		if inputset.LooksLikeLiquibaseRemoteRef(item) {
			return nil, inputset.Errorf("unsupported_resource", "external Liquibase search path is unsupported: %s", item)
		}
		path, err := resolver.ResolvePath(item)
		if err != nil {
			return nil, err
		}
		out = append(out, path)
	}
	return out, nil
}

type tracker struct {
	fs           inputset.FileSystem
	root         string
	searchPaths  []string
	seen         map[string]struct{}
	snapshots    map[string]string
	visitingDirs map[string]struct{}
}

func (t *tracker) addLeaf(path string, order *[]string) error {
	path = filepath.Clean(path)
	if err := t.validateFile(path); err != nil {
		return err
	}
	if _, ok := t.seen[path]; ok {
		return nil
	}
	t.seen[path] = struct{}{}
	*order = append(*order, path)
	return nil
}

func (t *tracker) collect(path string, order *[]string) error {
	path = filepath.Clean(path)
	if err := t.validateFile(path); err != nil {
		return err
	}
	if _, ok := t.seen[path]; ok {
		return nil
	}
	t.seen[path] = struct{}{}
	*order = append(*order, path)

	content, err := t.fs.ReadFile(path)
	if err != nil {
		return err
	}
	if t.snapshots == nil {
		t.snapshots = make(map[string]string)
	}
	t.snapshots[path] = inputset.HashContent(content)
	refs, err := parseChangelogRefs(path, content)
	if err != nil {
		return err
	}

	changelogDir := filepath.Dir(path)
	for _, ref := range refs.ordered {
		switch {
		case ref.include != nil:
			inc := ref.include
			if err := validateResourceRef(inc.File); err != nil {
				return err
			}
			next := t.resolveIncludePath(changelogDir, inc.File, inc.RelativeToChangelogFile.useChangelogDir())
			if err := t.collect(next, order); err != nil {
				return err
			}
		case ref.includeAll != nil:
			inc := ref.includeAll
			if err := validateResourceRef(inc.Path); err != nil {
				return err
			}
			dir := t.resolveIncludePath(changelogDir, inc.Path, inc.RelativeToChangelogFile.useChangelogDir())
			if err := t.collectIncludeAll(dir, order); err != nil {
				return err
			}
		case ref.localPath != nil:
			leaf := ref.localPath
			if err := validateResourceRef(leaf.rawPath()); err != nil {
				return err
			}
			next := t.resolveIncludePath(changelogDir, leaf.rawPath(), leaf.RelativeToChangelogFile.useChangelogDir())
			if err := t.addLeaf(next, order); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateFile prevents a path which is absent, non-regular, outside the
// admitted workspace, or an outbound host symlink from entering the manifest.
// The bundle requirements are in docs/architecture/liquibase-container-component-structure.md.
func (t *tracker) validateFile(path string) error {
	if err := t.validateWithinRoot(path); err != nil {
		return err
	}
	info, err := t.fs.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return inputset.Errorf("unsupported_resource", "Liquibase input must be a regular file: %s", path)
	}
	return nil
}

func (t *tracker) validateWithinRoot(path string) error {
	root, err := filepath.Abs(t.root)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, ok := t.fs.(inputset.OSFileSystem); ok {
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return inputset.Errorf("outside_workspace", "Liquibase input is outside workspace: %s", path)
	}
	return nil
}

func validateResourceRef(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return inputset.Errorf("unsupported_resource", "empty Liquibase resource reference")
	}
	if inputset.LooksLikeLiquibaseRemoteRef(value) || strings.Contains(value, "${") {
		return inputset.Errorf("unsupported_resource", "unsupported Liquibase resource reference: %s", value)
	}
	return nil
}

func (t *tracker) collectIncludeAll(dir string, order *[]string) error {
	if err := t.validateWithinRoot(dir); err != nil {
		return err
	}
	canonicalDir := filepath.Clean(dir)
	if _, ok := t.fs.(inputset.OSFileSystem); ok {
		if resolved, err := filepath.EvalSymlinks(canonicalDir); err == nil {
			canonicalDir = resolved
		}
	}
	if t.visitingDirs == nil {
		t.visitingDirs = make(map[string]struct{})
	}
	if _, active := t.visitingDirs[canonicalDir]; active {
		return inputset.Errorf("circular_include_all", "circular Liquibase includeAll directory: %s", dir)
	}
	t.visitingDirs[canonicalDir] = struct{}{}
	defer delete(t.visitingDirs, canonicalDir)
	entries, err := t.fs.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("includeAll %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
			continue
		}
		lower := strings.ToLower(entry.Name())
		if strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".sql") ||
			strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") ||
			strings.HasSuffix(lower, ".json") {
			names = append(names, entry.Name())
		} else {
			return inputset.Errorf("unsupported_resource", "unclassified Liquibase includeAll entry: %s", filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		info, err := t.fs.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = t.collectIncludeAll(path, order)
		} else {
			err = t.collect(path, order)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *tracker) resolveIncludePath(changelogDir string, raw string, relativeToChangelog bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if inputset.LooksLikeLiquibaseRemoteRef(raw) {
		return ""
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	if relativeToChangelog {
		return filepath.Clean(filepath.Join(changelogDir, filepath.FromSlash(raw)))
	}
	for _, base := range t.searchPaths {
		candidate := filepath.Clean(filepath.Join(base, filepath.FromSlash(raw)))
		if _, err := t.fs.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Clean(filepath.Join(t.root, filepath.FromSlash(raw)))
}

type changelogRefs struct {
	includes    []includeSpec
	includeAlls []includeAll
	localPaths  []localPathRef
	ordered     []orderedRef
}

type orderedRef struct {
	include    *includeSpec
	includeAll *includeAll
	localPath  *localPathRef
}

func parseChangelogRefs(path string, content []byte) (changelogRefs, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xml":
		var log xmlChangeLog
		if err := xml.Unmarshal(content, &log); err != nil {
			return changelogRefs{}, fmt.Errorf("parse liquibase changelog %s: %w", path, err)
		}
		ordered, err := parseXMLOrderedRefs(content)
		if err != nil {
			return changelogRefs{}, fmt.Errorf("parse liquibase changelog %s: %w", path, err)
		}
		return changelogRefs{
			includes:    log.Includes,
			includeAlls: log.IncludeAll,
			localPaths:  flattenXMLLocalPathRefs(log.ChangeSets),
			ordered:     ordered,
		}, nil
	case ".yaml", ".yml":
		var log structuredChangeLog
		if err := yaml.Unmarshal(content, &log); err != nil {
			return changelogRefs{}, fmt.Errorf("parse liquibase changelog %s: %w", path, err)
		}
		return changelogRefs{
			includes:    flattenIncludes(log.DatabaseChangeLog),
			includeAlls: flattenIncludeAll(log.DatabaseChangeLog),
			localPaths:  flattenLocalPathRefs(log.DatabaseChangeLog),
			ordered:     orderedStructuredRefs(log.DatabaseChangeLog),
		}, nil
	case ".json":
		var log structuredChangeLog
		if err := json.Unmarshal(content, &log); err != nil {
			return changelogRefs{}, fmt.Errorf("parse liquibase changelog %s: %w", path, err)
		}
		return changelogRefs{
			includes:    flattenIncludes(log.DatabaseChangeLog),
			includeAlls: flattenIncludeAll(log.DatabaseChangeLog),
			localPaths:  flattenLocalPathRefs(log.DatabaseChangeLog),
			ordered:     orderedStructuredRefs(log.DatabaseChangeLog),
		}, nil
	case ".sql":
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(string(content))), "--liquibase formatted sql") {
			return changelogRefs{}, inputset.Errorf("unsupported_changelog", "SQL changelog is not Liquibase formatted SQL: %s", path)
		}
		for _, raw := range strings.Split(string(content), "\n") {
			line := strings.ToLower(strings.TrimSpace(raw))
			if strings.HasPrefix(line, "--include ") || strings.HasPrefix(line, "--includeall ") {
				return changelogRefs{}, inputset.Errorf("unsupported_resource", "formatted SQL includes require unsupported Liquibase Secure features: %s", path)
			}
		}
		return changelogRefs{}, nil
	default:
		return changelogRefs{}, inputset.Errorf("unsupported_changelog", "unsupported Liquibase changelog extension: %s", path)
	}
}

func orderedStructuredRefs(items []changeItem) []orderedRef {
	var result []orderedRef
	for _, item := range items {
		if item.Include != nil {
			result = append(result, orderedRef{include: item.Include})
		}
		if item.IncludeAll != nil {
			result = append(result, orderedRef{includeAll: item.IncludeAll})
		}
		if item.ChangeSet != nil {
			for _, ref := range localPathRefsFromChangeSet(*item.ChangeSet) {
				copy := ref
				result = append(result, orderedRef{localPath: &copy})
			}
		}
	}
	return result
}

func parseXMLOrderedRefs(content []byte) ([]orderedRef, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var result []orderedRef
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		attribute := func(name string) string {
			for _, attr := range start.Attr {
				if attr.Name.Local == name {
					return attr.Value
				}
			}
			return ""
		}
		switch start.Name.Local {
		case "include":
			result = append(result, orderedRef{include: &includeSpec{File: attribute("file"), RelativeToChangelogFile: boolish(attribute("relativeToChangelogFile"))}})
		case "includeAll":
			if attribute("resourceComparator") != "" || attribute("filter") != "" {
				return nil, inputset.Errorf("unsupported_resource", "custom Liquibase includeAll loader or ordering is unsupported")
			}
			result = append(result, orderedRef{includeAll: &includeAll{Path: attribute("path"), RelativeToChangelogFile: boolish(attribute("relativeToChangelogFile"))}})
		case "sqlFile", "loadData", "loadUpdateData":
			result = append(result, orderedRef{localPath: &localPathRef{Path: attribute("path"), File: attribute("file"), RelativeToChangelogFile: boolish(attribute("relativeToChangelogFile"))}})
		case "customChange", "customPrecondition":
			return nil, inputset.Errorf("unsupported_resource", "custom Liquibase resource loader is unsupported: %s", start.Name.Local)
		}
	}
}

func parseIncludes(path string, content []byte) ([]includeSpec, []includeAll, error) {
	refs, err := parseChangelogRefs(path, content)
	if err != nil {
		return nil, nil, err
	}
	return refs.includes, refs.includeAlls, nil
}

func flattenIncludes(items []changeItem) []includeSpec {
	out := make([]includeSpec, 0, len(items))
	for _, item := range items {
		if item.Include != nil {
			out = append(out, *item.Include)
		}
	}
	return out
}

func flattenIncludeAll(items []changeItem) []includeAll {
	out := make([]includeAll, 0, len(items))
	for _, item := range items {
		if item.IncludeAll != nil {
			out = append(out, *item.IncludeAll)
		}
	}
	return out
}

func flattenXMLLocalPathRefs(changeSets []xmlChangeSet) []localPathRef {
	out := make([]localPathRef, 0, len(changeSets))
	for _, set := range changeSets {
		out = append(out, set.SQLFiles...)
		out = append(out, set.LoadData...)
		out = append(out, set.LoadUpdateData...)
	}
	return out
}

func flattenLocalPathRefs(items []changeItem) []localPathRef {
	out := make([]localPathRef, 0, len(items))
	for _, item := range items {
		if item.ChangeSet == nil {
			continue
		}
		out = append(out, localPathRefsFromChangeSet(*item.ChangeSet)...)
	}
	return out
}

func localPathRefsFromChangeSet(set changeSetSpec) []localPathRef {
	out := make([]localPathRef, 0, 3+len(set.Changes))
	if set.SQLFile != nil {
		out = append(out, *set.SQLFile)
	}
	if set.LoadData != nil {
		out = append(out, *set.LoadData)
	}
	if set.LoadUpdateData != nil {
		out = append(out, *set.LoadUpdateData)
	}
	for _, change := range set.Changes {
		if change.SQLFile != nil {
			out = append(out, *change.SQLFile)
		}
		if change.LoadData != nil {
			out = append(out, *change.LoadData)
		}
		if change.LoadUpdateData != nil {
			out = append(out, *change.LoadUpdateData)
		}
	}
	return out
}

func validateSearchPath(value string, resolver inputset.Resolver, fs inputset.FileSystem) []inputset.UserError {
	if strings.TrimSpace(value) == "" {
		return []inputset.UserError{*inputset.Errorf("empty_search_path", "searchPath is empty")}
	}
	parts := strings.Split(value, ",")
	issues := make([]inputset.UserError, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			issues = append(issues, *inputset.Errorf("empty_search_path_item", "searchPath is empty"))
			continue
		}
		if issue, ok := validateLocalArg(item, resolver, fs, false); ok {
			issues = append(issues, issue)
		}
	}
	return issues
}

func validateLocalArg(value string, resolver inputset.Resolver, fs inputset.FileSystem, requireFile bool) (inputset.UserError, bool) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return *inputset.Errorf("empty_path", "file path is empty"), true
	}
	if inputset.LooksLikeLiquibaseRemoteRef(cleaned) {
		return inputset.UserError{}, false
	}
	resolved, err := resolver.ResolvePath(cleaned)
	if err != nil {
		if issue, ok := err.(*inputset.UserError); ok {
			return *issue, true
		}
		return inputset.UserError{Code: "invalid_path", Message: err.Error()}, true
	}
	info, err := fs.Stat(resolved)
	if err != nil {
		return *inputset.Errorf("missing_path", "referenced path not found: %s", cleaned), true
	}
	if requireFile && info.IsDir() {
		return *inputset.Errorf("expected_file", "referenced path must be a file: %s", cleaned), true
	}
	return inputset.UserError{}, false
}
