package service

import (
	"os"
	"testing"

	"github.com/kenyamaneko/overload-party-shop/internal/repository/testutil"
)

var sharedPg *testutil.Postgres

func TestMain(m *testing.M) {
	os.Exit(testutil.RunMain(m, &sharedPg,
		testutil.WithSchemaFile("db/schema.sql"),
		testutil.WithSchema("shop"),
	))
}
