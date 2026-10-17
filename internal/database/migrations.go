package database

import "embed"

//go:embed migrations/*.sql
var migrationFiles embed.FS

func ReadMigration(name string) ([]byte, error) {
	return migrationFiles.ReadFile("migrations/" + name)
}
