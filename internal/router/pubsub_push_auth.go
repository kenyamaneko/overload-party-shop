package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/api/idtoken"
)

// googleOIDCIssuer は Google が発行する OIDC ID トークンの iss クレーム値。
const googleOIDCIssuer = "https://accounts.google.com"

// PubSubPushTokenValidator は Pub/Sub push リクエストに載る Google 発行の OIDC ID トークンを検証し、
// 有効なら署名者の email クレームを返す。
type PubSubPushTokenValidator interface {
	Validate(ctx context.Context, idToken, audience string) (email string, err error)
}

// idTokenValidateFunc は Google の公開鍵による署名・有効期限・audience の検証を抽象化する。
// 本番は idtoken.Validate を使う。
type idTokenValidateFunc func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error)

// googleIDTokenValidator は idtoken.Validate による検証に加え issuer と email_verified を
// 確認する PubSubPushTokenValidator の実装。
type googleIDTokenValidator struct {
	validate idTokenValidateFunc
}

// NewGoogleIDTokenValidator は本番用の PubSubPushTokenValidator を返す。
func NewGoogleIDTokenValidator() PubSubPushTokenValidator {
	return &googleIDTokenValidator{validate: idtoken.Validate}
}

var _ PubSubPushTokenValidator = (*googleIDTokenValidator)(nil)

// Validate は署名・有効期限・audience に加え issuer と email_verified を確認し、
// すべて満たせば email クレームを返す。
func (v *googleIDTokenValidator) Validate(ctx context.Context, idTok, audience string) (string, error) {
	payload, err := v.validate(ctx, idTok, audience)
	if err != nil {
		return "", fmt.Errorf("pubsub push auth: %w", err)
	}
	if payload.Issuer != googleOIDCIssuer {
		return "", fmt.Errorf("pubsub push auth: unexpected issuer %q", payload.Issuer)
	}
	emailVerified, _ := payload.Claims["email_verified"].(bool)
	if !emailVerified {
		return "", errors.New("pubsub push auth: email not verified")
	}
	email, _ := payload.Claims["email"].(string)
	if email == "" {
		return "", errors.New("pubsub push auth: email claim missing")
	}
	return email, nil
}

// verifyPubSubPush は Pub/Sub push リクエストの Authorization: Bearer <OIDC ID トークン> を検証する
// Gin middleware を返す。署名・有効期限・issuer・audience・push 用サービスアカウントの email の
// いずれかを満たさなければ 401 で後続の handler (usecase) には到達させない。
func verifyPubSubPush(validator PubSubPushTokenValidator, expectedEmail, audience string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		idTok := strings.TrimPrefix(authHeader, "Bearer ")
		if idTok == authHeader {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			return
		}

		email, err := validator.Validate(c.Request.Context(), idTok, audience)
		if err != nil {
			slog.WarnContext(c.Request.Context(), "pubsub push auth: token validation failed",
				slog.String("error", err.Error()),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		if email != expectedEmail {
			slog.WarnContext(c.Request.Context(), "pubsub push auth: unexpected service account",
				slog.String("email", email),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unexpected service account"})
			return
		}
		c.Next()
	}
}
