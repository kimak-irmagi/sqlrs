package aliasruntimev2

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/composition"
	legacyalias "github.com/sqlrs/cli/internal/alias"
	"github.com/sqlrs/cli/internal/pathutil"
)

const (
	MaxCompatibilityMessageBytes = runtimev2.MaxResolvedValueBytes
	MaxSourceIDBytes             = composition.MaxSourceIDBytes
)

// Status identifies the one exclusive compatibility outcome.
type Status string

const (
	StatusTranslated Status = "translated"
	StatusLegacyOnly Status = "legacy_only"
)

// ReasonCode is a stable machine-readable legacy-only classification.
type ReasonCode string

const (
	ReasonProviderUnavailable ReasonCode = "runtime_v2_provider_unavailable"
	ReasonDefaultUnavailable  ReasonCode = "legacy_default_unavailable"
	ReasonUnsupported         ReasonCode = "legacy_semantics_unsupported"
)

// ImageSource identifies which already-applied legacy precedence rule supplied
// the effective image. It is diagnostic and never enters Runtime v2 identity.
type ImageSource string

const (
	ImageSourceAlias           ImageSource = "alias"
	ImageSourceWorkspaceConfig ImageSource = "workspace_config"
	ImageSourceGlobalConfig    ImageSource = "global_config"
)

// TranslationInput is the fully bound, workspace-checked legacy alias supplied
// to a provider-aware declaration adapter.
type TranslationInput struct {
	Definition           legacyalias.Definition
	WorkspaceRoot        string
	AliasPath            string
	SourceID             string
	EffectiveImage       string
	EffectiveImageSource ImageSource
}

// ProviderAdapter converts one complete supported legacy prepare definition to
// a deterministic Runtime v2 recipe declaration or an actionable legacy-only
// result. Ordinary unsupported forms are results rather than errors.
type ProviderAdapter interface {
	Kind() string
	Translate(TranslationInput) (Result, error)
}

// Result is an immutable exclusive translated-or-legacy-only outcome.
type Result struct {
	status      Status
	declaration runtimev2.RecipeDeclaration
	reason      ReasonCode
	message     string
}

// NewTranslatedResult validates and snapshots a complete recipe declaration.
func NewTranslatedResult(declaration runtimev2.RecipeDeclaration) (Result, error) {
	copy, err := copyRecipe(declaration)
	if err != nil {
		return Result{}, fmt.Errorf("invalid translated Runtime v2 declaration: %w", err)
	}
	return Result{status: StatusTranslated, declaration: copy}, nil
}

// NewLegacyOnlyResult constructs an actionable result for a workflow that must
// remain on the current legacy execution path.
func NewLegacyOnlyResult(reason ReasonCode, message string) (Result, error) {
	if !validReason(reason) {
		return Result{}, errors.New("unknown Runtime v2 legacy-only reason")
	}
	if message == "" || !utf8.ValidString(message) || len(message) > MaxCompatibilityMessageBytes {
		return Result{}, errors.New("Runtime v2 legacy-only message is invalid")
	}
	return Result{status: StatusLegacyOnly, reason: reason, message: message}, nil
}

// Status returns the exclusive compatibility outcome.
func (r Result) Status() Status { return r.status }

// Declaration returns an immutable declaration snapshot only for translated
// results.
func (r Result) Declaration() (runtimev2.RecipeDeclaration, bool) {
	if r.status != StatusTranslated {
		return runtimev2.RecipeDeclaration{}, false
	}
	// RecipeDeclaration is an opaque immutable value whose own accessors return
	// defensive copies, so returning the value cannot expose mutable state.
	return r.declaration, true
}

// Reason returns the stable legacy-only reason or an empty value for a
// translated result.
func (r Result) Reason() ReasonCode { return r.reason }

// Message returns the bounded actionable legacy-only remedy.
func (r Result) Message() string { return r.message }

// TranslateLegacy enforces the common path/default/result contract before and
// after invoking one provider-aware adapter. It never selects Runtime v2
// execution or mutates legacy records.
func TranslateLegacy(input TranslationInput, adapter ProviderAdapter) (Result, error) {
	validated, err := validateTranslationInput(input)
	if err != nil {
		return Result{}, err
	}
	if validated.Definition.Class == legacyalias.ClassRun {
		return NewLegacyOnlyResult(ReasonUnsupported, "run aliases remain on the legacy executor because their semantics are not Runtime v2 transforms")
	}
	if nilInterface(adapter) {
		return NewLegacyOnlyResult(ReasonProviderUnavailable, "no Runtime v2 provider adapter is available for this prepare alias")
	}
	if validated.EffectiveImage == "" {
		return NewLegacyOnlyResult(ReasonDefaultUnavailable, "select an explicit database image before translating this prepare alias")
	}
	if adapter.Kind() != validated.Definition.Kind {
		return Result{}, errors.New("Runtime v2 provider adapter kind does not match the legacy alias kind")
	}

	result, err := adapter.Translate(validated)
	if err != nil {
		return Result{}, err
	}
	if err := validateResult(result); err != nil {
		return Result{}, fmt.Errorf("invalid Runtime v2 provider adapter result: %w", err)
	}
	return cloneResult(result)
}

func validateTranslationInput(input TranslationInput) (TranslationInput, error) {
	if input.Definition.Class != legacyalias.ClassPrepare && input.Definition.Class != legacyalias.ClassRun {
		return TranslationInput{}, errors.New("legacy alias class must be prepare or run")
	}
	if input.Definition.Kind == "" || input.Definition.Kind != strings.TrimSpace(input.Definition.Kind) || input.Definition.Kind != strings.ToLower(input.Definition.Kind) {
		return TranslationInput{}, errors.New("legacy alias kind must be a normalized lower-case value")
	}
	if !canonicalAbsolutePath(input.WorkspaceRoot) || !canonicalAbsolutePath(input.AliasPath) {
		return TranslationInput{}, errors.New("workspace root and alias path must be canonical absolute paths")
	}
	canonicalRoot := pathutil.CanonicalizeBoundaryPath(input.WorkspaceRoot)
	canonicalAlias := pathutil.CanonicalizeBoundaryPath(input.AliasPath)
	if !pathutil.IsWithin(canonicalRoot, canonicalAlias) || pathutil.SameLocalPath(canonicalRoot, canonicalAlias) {
		return TranslationInput{}, errors.New("alias path must be strictly inside the workspace root")
	}
	// IsWithin already performed the same filepath.Rel operation and rejected a
	// cross-volume or otherwise non-relativizable pair.
	relative, _ := filepath.Rel(canonicalRoot, canonicalAlias)
	expectedSourceID := filepath.ToSlash(relative)
	if !validSourceID(input.SourceID) || input.SourceID != expectedSourceID {
		return TranslationInput{}, errors.New("source ID must equal the canonical workspace-relative alias path")
	}
	if input.Definition.Image != strings.TrimSpace(input.Definition.Image) || input.EffectiveImage != strings.TrimSpace(input.EffectiveImage) {
		return TranslationInput{}, errors.New("legacy image values must already be normalized")
	}
	if err := validateImageSelection(input); err != nil {
		return TranslationInput{}, err
	}

	input.Definition.Args = append([]string(nil), input.Definition.Args...)
	return input, nil
}

func canonicalAbsolutePath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func validSourceID(value string) bool {
	if value == "" || len(value) > MaxSourceIDBytes || !utf8.ValidString(value) || strings.Contains(value, `\`) || filepath.IsAbs(value) || path.Clean(value) != value {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func validateImageSelection(input TranslationInput) error {
	switch {
	case input.Definition.Class == legacyalias.ClassRun:
		if input.Definition.Image != "" || input.EffectiveImage != "" || input.EffectiveImageSource != "" {
			return errors.New("legacy run aliases cannot carry an effective database image")
		}
	case input.Definition.Image != "":
		if input.EffectiveImage != input.Definition.Image || input.EffectiveImageSource != ImageSourceAlias {
			return errors.New("explicit legacy image and effective image provenance are inconsistent")
		}
	case input.EffectiveImage != "":
		if input.EffectiveImageSource != ImageSourceWorkspaceConfig && input.EffectiveImageSource != ImageSourceGlobalConfig {
			return errors.New("inherited legacy image provenance is invalid")
		}
	default:
		if input.EffectiveImageSource != "" {
			return errors.New("an absent effective image cannot have provenance")
		}
	}
	return nil
}

func validateResult(result Result) error {
	switch result.status {
	case StatusTranslated:
		if result.reason != "" || result.message != "" {
			return errors.New("translated result also contains legacy-only fields")
		}
		_, err := copyRecipe(result.declaration)
		return err
	case StatusLegacyOnly:
		if _, err := copyRecipe(result.declaration); err == nil {
			return errors.New("legacy-only result also contains a declaration")
		}
		if !validReason(result.reason) || result.message == "" || !utf8.ValidString(result.message) || len(result.message) > MaxCompatibilityMessageBytes {
			return errors.New("legacy-only result is incomplete")
		}
		return nil
	default:
		return errors.New("compatibility result status is invalid")
	}
}

func cloneResult(result Result) (Result, error) {
	if result.status == StatusTranslated {
		return NewTranslatedResult(result.declaration)
	}
	return NewLegacyOnlyResult(result.reason, result.message)
}

func copyRecipe(value runtimev2.RecipeDeclaration) (runtimev2.RecipeDeclaration, error) {
	return runtimev2.NewRecipeDeclaration(value.Factory(), value.Transforms())
}

func validReason(reason ReasonCode) bool {
	switch reason {
	case ReasonProviderUnavailable, ReasonDefaultUnavailable, ReasonUnsupported:
		return true
	default:
		return false
	}
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
