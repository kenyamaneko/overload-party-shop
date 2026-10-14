package configtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/config"
)

// EnvBuilder は、上書きする環境変数から、起動設定を読み込む環境変数一式を組み立てる。
type EnvBuilder func(overrides map[string]string) map[string]string

// ReadCase は、環境変数を上書きして読み込んだ起動設定の値を確かめるテーブル駆動の 1 行である。
type ReadCase struct {
	Name string
	Env  map[string]string
	Read func(cfg *config.Config) any
	Want any
}

// ErrorCase は、環境変数を上書きして起動設定の読み込みが返すエラーの内容を確かめるテーブル駆動の 1 行である。
type ErrorCase struct {
	Name         string
	Env          map[string]string
	WantContains []string
}

// LoadConfig は env を環境変数に設定して起動設定を読み込む。
func LoadConfig(t *testing.T, env map[string]string) (*config.Config, error) {
	t.Helper()
	SetEnv(t, env)
	return config.FromEnv()
}

// AssertConfigError は env で起動設定を読み込むとエラーになり、その内容が wantContains をすべて含むことを確かめる。
func AssertConfigError(t *testing.T, env map[string]string, wantContains []string) {
	t.Helper()
	_, err := LoadConfig(t, env)
	require.Error(t, err)
	for _, want := range wantContains {
		assert.ErrorContains(t, err, want)
	}
}

// RunReadCases は cases の各行を、build で組み立てた環境変数で読み込んだ起動設定の値を確かめるサブテストとして実行する。
func RunReadCases(t *testing.T, build EnvBuilder, cases []ReadCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			cfg, err := LoadConfig(t, build(tt.Env))
			require.NoError(t, err)
			assert.Equal(t, tt.Want, tt.Read(cfg))
		})
	}
}

// RunErrorCases は cases の各行を、build で組み立てた環境変数で読み込むと返るエラーの内容を確かめるサブテストとして実行する。
func RunErrorCases(t *testing.T, build EnvBuilder, cases []ErrorCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			AssertConfigError(t, build(tt.Env), tt.WantContains)
		})
	}
}
