// Package db provides database connection and utilities.
package db

import (
	"encore.dev/storage/sqldb"
)

// ProtisanDB is the global database connection used by services in this app.
var ProtisanDB = sqldb.NewDatabase("protisan", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})