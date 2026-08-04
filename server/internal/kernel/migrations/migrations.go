// Package migrations embebe el esquema del kernel.
//
// Es su propio paquete y no parte de `db` porque go:embed solo alcanza al
// directorio del archivo que lo declara, y el SQL tiene que vivir junto a las
// tablas que describe, no junto al runner que las aplica.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

// FS son los .sql del kernel, listos para db.Migrate.
func FS() fs.FS {
	sub, err := fs.Sub(files, ".")
	if err != nil {
		panic(fmt.Sprintf("kernel: migraciones embebidas ilegibles: %v", err))
	}
	return sub
}
