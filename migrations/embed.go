package migrations

import "embed"

// Files contains the embedded License Service PostgreSQL migrations.
//
//go:embed 00001_activation_records.up.sql
var Files embed.FS
