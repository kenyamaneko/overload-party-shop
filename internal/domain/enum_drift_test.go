package domain_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/kenyamaneko/overload-party-shop/internal/domain"
)

const openAPISpecPath = "../../data/openapi.yaml"

type openAPIDocument struct {
	Components struct {
		Schemas map[string]struct {
			Enum []string `yaml:"enum"`
		} `yaml:"schemas"`
	} `yaml:"components"`
}

func loadSchemaEnumValues(t *testing.T, schemaName string) []string {
	t.Helper()
	raw, err := os.ReadFile(openAPISpecPath)
	require.NoError(t, err)

	var doc openAPIDocument
	require.NoError(t, yaml.Unmarshal(raw, &doc))

	schema, found := doc.Components.Schemas[schemaName]
	require.True(t, found, "components/schemas に %s が無い", schemaName)
	return schema.Enum
}

func TestAPIContractEnumValues(t *testing.T) {
	t.Run("[API契約]商品種別とクライアント実行環境の値の整合", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			cases := []struct {
				name       string
				schemaName string
				handled    []string
			}{
				{
					name:       "アプリが扱う商品種別の値の集合は、openapi.yamlのProductTypeの列挙値の集合と過不足なく一致する",
					schemaName: "ProductType",
					handled: []string{
						domain.ProductTypeFactionSet,
						domain.ProductTypeCardPack,
						domain.ProductTypeCosmetic,
						domain.ProductTypeSubscription,
					},
				},
				{
					name:       "アプリが扱うクライアント実行環境の値の集合は、openapi.yamlのPlatformの列挙値の集合と過不足なく一致する",
					schemaName: "Platform",
					handled: []string{
						domain.PlatformIOS,
						domain.PlatformAndroid,
					},
				},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					assert.ElementsMatch(t, loadSchemaEnumValues(t, tc.schemaName), tc.handled)
				})
			}
		})
	})
}
