package router

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"
)

func TestGoogleIDTokenValidatorValidate(t *testing.T) {
	t.Run("Pub/Sub pushトークンの正当性判定", func(t *testing.T) {
		validClaims := map[string]interface{}{"email_verified": true, "email": "sa@example.com"}

		errorCases := []struct {
			name          string
			validateFn    idTokenValidateFunc
			wantErrSubstr string
		}{
			{
				name: "トークン自体の署名・有効期限などの基礎検証が失敗するとき、検証は失敗と判定される",
				validateFn: func(context.Context, string, string) (*idtoken.Payload, error) {
					return nil, errors.New("signature invalid")
				},
				wantErrSubstr: "signature invalid",
			},
			{
				name: "基礎検証は通るが、発行者の情報がGoogleの発行者情報と一致しないとき、検証は失敗と判定される",
				validateFn: func(context.Context, string, string) (*idtoken.Payload, error) {
					return &idtoken.Payload{Issuer: "https://evil.example", Claims: validClaims}, nil
				},
				wantErrSubstr: "unexpected issuer",
			},
			{
				name: "発行者情報は一致するが、メールアドレスが検証済みであることを示すクレームが真でないとき (クレーム自体が存在しない場合)、検証は失敗と判定される",
				validateFn: func(context.Context, string, string) (*idtoken.Payload, error) {
					return &idtoken.Payload{Issuer: googleOIDCIssuer, Claims: map[string]interface{}{"email": "sa@example.com"}}, nil
				},
				wantErrSubstr: "email not verified",
			},
			{
				name: "発行者情報は一致するが、メールアドレスが検証済みであることを示すクレームが真でないとき (クレームがfalseの場合)、検証は失敗と判定される",
				validateFn: func(context.Context, string, string) (*idtoken.Payload, error) {
					return &idtoken.Payload{Issuer: googleOIDCIssuer, Claims: map[string]interface{}{"email_verified": false, "email": "sa@example.com"}}, nil
				},
				wantErrSubstr: "email not verified",
			},
			{
				name: "発行者情報とメール検証済みクレームは満たすが、メールアドレスのクレームが存在しないとき、検証は失敗と判定される",
				validateFn: func(context.Context, string, string) (*idtoken.Payload, error) {
					return &idtoken.Payload{Issuer: googleOIDCIssuer, Claims: map[string]interface{}{"email_verified": true}}, nil
				},
				wantErrSubstr: "email claim missing",
			},
			{
				name: "発行者情報とメール検証済みクレームは満たすが、メールアドレスのクレームが空のとき、検証は失敗と判定される",
				validateFn: func(context.Context, string, string) (*idtoken.Payload, error) {
					return &idtoken.Payload{Issuer: googleOIDCIssuer, Claims: map[string]interface{}{"email_verified": true, "email": ""}}, nil
				},
				wantErrSubstr: "email claim missing",
			},
		}

		for _, tc := range errorCases {
			t.Run(tc.name, func(t *testing.T) {
				v := &googleIDTokenValidator{validate: tc.validateFn}

				email, err := v.Validate(context.Background(), "id-token", "audience")

				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrSubstr)
				assert.Equal(t, "", email)
			})
		}

		t.Run("発行者情報・メール検証済み・メールアドレスのすべての条件を満たすとき、検証は成功し、そのメールアドレスが判定結果として返る", func(t *testing.T) {
			v := &googleIDTokenValidator{validate: func(context.Context, string, string) (*idtoken.Payload, error) {
				return &idtoken.Payload{Issuer: googleOIDCIssuer, Claims: validClaims}, nil
			}}

			email, err := v.Validate(context.Background(), "id-token", "audience")

			require.NoError(t, err)
			assert.Equal(t, "sa@example.com", email)
		})
	})
}
