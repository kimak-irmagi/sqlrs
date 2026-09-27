package sqlite

import (
	_ "embed"
	"strings"
)

const (
	runtimeV2SchemaBegin = "-- BEGIN SQLRS RUNTIME V2 SCHEMA"
	runtimeV2SchemaEnd   = "-- END SQLRS RUNTIME V2 SCHEMA"
)

//go:embed schema.sql
var schemaSQL string

func SchemaSQL() string {
	start := strings.Index(schemaSQL, runtimeV2SchemaBegin)
	if start < 0 {
		return schemaSQL
	}
	return strings.TrimSpace(schemaSQL[:start])
}

// RuntimeV2SchemaSQL returns the Runtime v2 DDL extracted from the canonical
// schema.sql source. Requirements: runtime-v2-persistence-structure.md.
func RuntimeV2SchemaSQL() string {
	start := strings.Index(schemaSQL, runtimeV2SchemaBegin)
	end := strings.Index(schemaSQL, runtimeV2SchemaEnd)
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	start += len(runtimeV2SchemaBegin)
	return strings.TrimSpace(schemaSQL[start:end])
}
