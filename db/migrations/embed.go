package migrations

import "embed"

// Files contains the embedded SQL migration files shipped with the service.
//
//go:embed *.sql
var Files embed.FS
