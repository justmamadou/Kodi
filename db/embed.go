// Package dbschema embarque les migrations dans le binaire : déployer l'API suffit pour migrer.
package dbschema

import "embed"

// Migrations contient les fichiers golang-migrate.
//
//go:embed migrations/*.sql
var Migrations embed.FS
