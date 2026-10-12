//go:build integration

package purchase_test

import (
	"os"
	"testing"

	"github.com/kenyamaneko/overload-party-shop/internal/repository/postgres/postgrestest"
)

var pg *postgrestest.Postgres

func TestMain(m *testing.M) {
	os.Exit(postgrestest.RunShopMain(m, &pg))
}
