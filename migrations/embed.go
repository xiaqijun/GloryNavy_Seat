package migrations

import "embed"

// Files contains reviewed, versioned business-schema migrations.
//
//go:embed *.sql
var Files embed.FS
