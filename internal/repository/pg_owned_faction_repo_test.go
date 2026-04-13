package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/repository"
)

func TestPgOwnedFactionRepository_AddAndList(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgOwnedFactionRepository(sharedPg.Pool)
	ctx := context.Background()

	playerID := "11111111-aaaa-aaaa-aaaa-111111111111"

	require.NoError(t, repo.Add(ctx, playerID, "SHE"))
	require.NoError(t, repo.Add(ctx, playerID, "Tenki"))

	got, err := repo.List(ctx, playerID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"SHE", "Tenki"}, got)
}

func TestPgOwnedFactionRepository_Add_Idempotent(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgOwnedFactionRepository(sharedPg.Pool)
	ctx := context.Background()

	playerID := "22222222-bbbb-bbbb-bbbb-222222222222"

	require.NoError(t, repo.Add(ctx, playerID, "Sugar"))
	require.NoError(t, repo.Add(ctx, playerID, "Sugar"), "ON CONFLICT DO NOTHING で重複は無視される")

	got, err := repo.List(ctx, playerID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Sugar"}, got)
}

func TestPgOwnedFactionRepository_List_ScopesByPlayer(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgOwnedFactionRepository(sharedPg.Pool)
	ctx := context.Background()

	p1 := "33333333-cccc-cccc-cccc-333333333333"
	p2 := "44444444-dddd-dddd-dddd-444444444444"

	require.NoError(t, repo.Add(ctx, p1, "SHE"))
	require.NoError(t, repo.Add(ctx, p1, "Tuners"))
	require.NoError(t, repo.Add(ctx, p2, "Tenki"))

	got1, err := repo.List(ctx, p1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"SHE", "Tuners"}, got1)

	got2, err := repo.List(ctx, p2)
	require.NoError(t, err)
	assert.Equal(t, []string{"Tenki"}, got2)
}

func TestPgOwnedFactionRepository_List_Empty(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgOwnedFactionRepository(sharedPg.Pool)
	ctx := context.Background()

	got, err := repo.List(ctx, "55555555-eeee-eeee-eeee-555555555555")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestPgOwnedFactionRepository_Add_RejectsInvalidFaction(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgOwnedFactionRepository(sharedPg.Pool)
	ctx := context.Background()

	err := repo.Add(ctx, "66666666-ffff-ffff-ffff-666666666666", "InvalidFaction")
	require.Error(t, err, "CHECK 制約により拒否されること")
}
