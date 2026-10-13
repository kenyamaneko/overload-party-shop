package router

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"
)

func buildPayloadValidation(issuer string, claims map[string]any) idTokenValidateFunc {
	return func(context.Context, string, string) (*idtoken.Payload, error) {
		return &idtoken.Payload{Issuer: issuer, Claims: claims}, nil
	}
}

func TestGoogleIDTokenValidator(t *testing.T) {
	t.Run("[GooglePlay通知]OIDCトークンの正当性判定", func(t *testing.T) {
		t.Run("正常系", func(t *testing.T) {
			t.Run("発行者がhttps://accounts.google.comで、email_verifiedがtrueで、emailがsa@example.comのとき、検証結果のメールアドレスはsa@example.comになる", func(t *testing.T) {
				validator := &googleIDTokenValidator{validate: buildPayloadValidation(
					"https://accounts.google.com",
					map[string]any{"email": "sa@example.com", "email_verified": true},
				)}

				email, err := validator.Validate(context.Background(), "TST-ID-TOKEN", "TST-AUDIENCE")

				require.NoError(t, err)
				assert.Equal(t, "sa@example.com", email)
			})
		})

		t.Run("異常系", func(t *testing.T) {
			t.Run("署名・有効期限・audienceの検証の成功、発行者https://accounts.google.com、email_verifiedがtrue、emailがsa@example.comのうち、一つだけが満たされないとき", func(t *testing.T) {
				tests := []struct {
					name            string
					validate        idTokenValidateFunc
					wantErrContains string
				}{
					{
						name: "署名・有効期限・audienceの検証が失敗するとき、エラーの内容にその失敗の原因が含まれる",
						validate: func(context.Context, string, string) (*idtoken.Payload, error) {
							return nil, errors.New("TST-SIGNATURE-INVALID")
						},
						wantErrContains: "TST-SIGNATURE-INVALID",
					},
					{
						name: "発行者がhttps://evil.exampleのとき、エラーの内容に発行者が想定外であることを示す語が含まれる",
						validate: buildPayloadValidation(
							"https://evil.example",
							map[string]any{"email": "sa@example.com", "email_verified": true},
						),
						wantErrContains: "unexpected issuer",
					},
					{
						name: "email_verifiedのクレームが無いとき、エラーの内容にメールが未検証であることを示す語が含まれる",
						validate: buildPayloadValidation(
							"https://accounts.google.com",
							map[string]any{"email": "sa@example.com"},
						),
						wantErrContains: "email not verified",
					},
					{
						name: "email_verifiedがfalseのとき、エラーの内容にメールが未検証であることを示す語が含まれる",
						validate: buildPayloadValidation(
							"https://accounts.google.com",
							map[string]any{"email": "sa@example.com", "email_verified": false},
						),
						wantErrContains: "email not verified",
					},
					{
						name: "emailのクレームが無いとき、エラーの内容にメールアドレスのクレームが無いことを示す語が含まれる",
						validate: buildPayloadValidation(
							"https://accounts.google.com",
							map[string]any{"email_verified": true},
						),
						wantErrContains: "email claim missing",
					},
					{
						name: "emailが空文字のとき、エラーの内容にメールアドレスのクレームが無いことを示す語が含まれる",
						validate: buildPayloadValidation(
							"https://accounts.google.com",
							map[string]any{"email": "", "email_verified": true},
						),
						wantErrContains: "email claim missing",
					},
				}
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						validator := &googleIDTokenValidator{validate: tt.validate}

						email, err := validator.Validate(context.Background(), "TST-ID-TOKEN", "TST-AUDIENCE")

						require.Error(t, err)
						assert.Contains(t, err.Error(), tt.wantErrContains)
						assert.Empty(t, email)
					})
				}
			})
		})
	})
}
