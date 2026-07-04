// Package migrations embeds the ordered SQL schema migrations so the server
// binary is fully self-contained: the migration runner reads them from here,
// never from an on-disk source tree the deployed binary does not have.
package migrations

import "embed"

//go:embed *.up.sql
var FS embed.FS
