package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/repository"
)

func TestPgOwnedFactionRepository_Add(t *testing.T) {
	repo := repository.NewPgOwnedFactionRepository(sharedPg.Pool)
	ctx := context.Background()

	const (
		user1 = "11111111-1111-1111-1111-111111111111"
		user2 = "22222222-2222-2222-2222-222222222222"
	)

	type seed struct{ playerID, faction string }

	tests := []struct {
		name    string
		seeds   []seed
		player  string
		faction string
		wantErr bool
	}{
		{
			name:    "同じユーザーに重複するfactionはPK違反でエラー",
			seeds:   []seed{{user1, "SHE"}},
			player:  user1,
			faction: "SHE",
			wantErr: true,
		},
		{
			name:    "同じユーザーに重複しないfactionは追加成功",
			seeds:   []seed{{user1, "SHE"}},
			player:  user1,
			faction: "Tenki",
			wantErr: false,
		},
		{
			name:    "異なるユーザーに同じfactionは追加成功",
			seeds:   []seed{{user1, "SHE"}},
			player:  user2,
			faction: "SHE",
			wantErr: false,
		},
		{
			name:    "不正なfactionはCHECK制約でエラー",
			player:  user1,
			faction: "InvalidFaction",
			wantErr: true,
		},
		{
			name:    "player_IDが空文字(UUID不正)はエラー",
			player:  "",
			faction: "SHE",
			wantErr: true,
		},
		{
			name:    "factionが空文字はCHECK制約でエラー",
			player:  user1,
			faction: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sharedPg.Truncate(t)
			for _, s := range tt.seeds {
				require.NoError(t, repo.Add(ctx, s.playerID, s.faction))
			}

			err := repo.Add(ctx, tt.player, tt.faction)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPgOwnedFactionRepository_List(t *testing.T) {
	sharedPg.Truncate(t)
	repo := repository.NewPgOwnedFactionRepository(sharedPg.Pool)
	ctx := context.Background()

	const (
		userA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		userB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		userC = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)

	require.NoError(t, repo.Add(ctx, userA, "SHE"))
	require.NoError(t, repo.Add(ctx, userB, "SHE"))
	require.NoError(t, repo.Add(ctx, userB, "Tenki"))

	tests := []struct {
		name    string
		player  string
		want    []string
		wantErr bool
	}{
		{
			name:   "userAはSHEのみ取得",
			player: userA,
			want:   []string{"SHE"},
		},
		{
			name:   "userBはSHEとTenkiを取得",
			player: userB,
			want:   []string{"SHE", "Tenki"},
		},
		{
			name:   "所有行のないuserCは空",
			player: userC,
			want:   nil,
		},
		{
			name:    "player_IDが空文字(UUID不正)はエラー",
			player:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.List(ctx, tt.player)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
