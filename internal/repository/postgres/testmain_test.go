package postgres_test

import (
	"os"
	"testing"

	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

var sharedPg *postgrestest.Postgres

func TestMain(m *testing.M) {
	os.Exit(postgrestest.RunMain(m, &sharedPg,
		postgrestest.WithSchemaFile("db/schema.sql"),
		postgrestest.WithSchema("shop"),
	))
}
