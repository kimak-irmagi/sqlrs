package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/sqlrs/cli/internal/client"
)

// writeAuthProviders sorts a copy and emits only public summary fields, as
// specified in docs/architecture/cli-auth-provider-discovery-structure.md.
func writeAuthProviders(w io.Writer, list client.AuthProviderList, output string) error {
	providers := append([]client.AuthProviderSummary(nil), list.Providers...)
	sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })
	if output == "json" {
		return writeJSON(w, client.AuthProviderList{Providers: providers})
	}
	if _, err := fmt.Fprintln(w, "Available login providers:"); err != nil {
		return err
	}
	for _, provider := range providers {
		name := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, provider.DisplayName)
		if _, err := fmt.Fprintf(w, "  %s — %s\n    sqlrs auth login %s\n", provider.ID, name, provider.ID); err != nil {
			return err
		}
	}
	return nil
}
