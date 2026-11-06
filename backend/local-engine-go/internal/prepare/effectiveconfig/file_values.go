package effectiveconfig

import "io"

// ParseFileBearingProperties applies the narrow source adapter approved in
// liquibase-search-path-component-amendment.md §2. Admission still requires
// valid UTF-8, supported escapes and unprotected keys. Only file-bearing values
// use Java InputStream byte mapping followed by Java ASCII trim; ordinary
// settings retain ParseProperties' existing semantics.
func ParseFileBearingProperties(reader io.Reader) (map[string]string, error) {
	return parseProperties(reader, true)
}

// fileBearingProperty keeps native byte mapping away from ordinary properties.
func fileBearingProperty(key string) bool {
	switch key {
	case "searchPath", "liquibase.searchPath", "changeLogFile", "changelogFile", "liquibase.changeLogFile", "liquibase.changelogFile", "liquibase.command.changeLogFile", "liquibase.command.changelogFile":
		return true
	}
	return false
}

// SelectedSearchPath selects the first nonempty source-normalized alias. An
// empty prefixed assignment does not suppress a nonempty unprefixed value;
// absent/empty values inherit. The caller separately applies CLI precedence.
func SelectedSearchPath(properties map[string]string) string {
	if value := properties["liquibase.searchPath"]; value != "" {
		return value
	}
	return properties["searchPath"]
}
