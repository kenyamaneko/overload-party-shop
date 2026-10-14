// Package subscriptiontest はサブスクリプション通知の結合テストが共有する、通知の組み立てと外部ストアの検証の偽物を提供する。
package subscriptiontest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kenyamaneko/overload-party-shop/internal/port"
	"github.com/kenyamaneko/overload-party-shop/internal/port/porttest"
)

// AppleNotification は Apple App Store Server Notifications V2 の通知の、処理に関わる項目。
type AppleNotification struct {
	NotificationType      string
	Subtype               string
	BundleID              string
	OriginalTransactionID string
	// ExpiresAtMillis が nil のとき、取引情報から有効期限の項目を省く。
	ExpiresAtMillis *int64
}

// AppleSigner は署名付き文字列の発行と、その署名検証の偽物を担う。署名検証は、発行した文字列に対してだけ成功する。
type AppleSigner struct {
	t        *testing.T
	verifier *porttest.MockAppleJWSVerifier
	payloads map[string][]byte
	failures map[string]error
}

// NewAppleSigner は、何も発行していない AppleSigner を返す。
func NewAppleSigner(t *testing.T) *AppleSigner {
	signer := &AppleSigner{t: t, payloads: map[string][]byte{}, failures: map[string]error{}}
	signer.verifier = &porttest.MockAppleJWSVerifier{VerifyFn: signer.verify}
	return signer
}

// Verifier は発行した署名付き文字列を検証する偽物を返す。
func (s *AppleSigner) Verifier() port.AppleJWSVerifier {
	return s.verifier
}

// Sign は署名検証に成功して payload を返す署名付き文字列を発行する。
func (s *AppleSigner) Sign(payload []byte) string {
	signed := s.nextSigned()
	s.payloads[signed] = payload
	return signed
}

// SignInvalid は署名検証が err で失敗する署名付き文字列を発行する。
func (s *AppleSigner) SignInvalid(err error) string {
	signed := s.nextSigned()
	s.failures[signed] = err
	return signed
}

// SignNotification は取引情報も署名検証に成功する、通知全体の署名付き文字列を発行する。
func (s *AppleSigner) SignNotification(notification AppleNotification) string {
	s.t.Helper()
	return s.SignNotificationWithTransactionInfo(notification, s.Sign(s.marshal(buildTransactionInfo(notification))))
}

// SignNotificationWithTransactionInfo は取引情報に signedTransactionInfo を載せた、通知全体の署名付き文字列を発行する。
func (s *AppleSigner) SignNotificationWithTransactionInfo(notification AppleNotification, signedTransactionInfo string) string {
	s.t.Helper()
	body := map[string]any{
		"notificationType": notification.NotificationType,
		"subtype":          notification.Subtype,
		"data": map[string]any{
			"bundleId":              notification.BundleID,
			"signedTransactionInfo": signedTransactionInfo,
		},
	}
	return s.Sign(s.marshal(body))
}

func (s *AppleSigner) verify(signed string) ([]byte, error) {
	if err, isInvalid := s.failures[signed]; isInvalid {
		return nil, err
	}
	payload, isIssued := s.payloads[signed]
	if !isIssued {
		return nil, fmt.Errorf("署名付き文字列 %s は発行されていない", signed)
	}
	return payload, nil
}

func (s *AppleSigner) nextSigned() string {
	return fmt.Sprintf("TST-SIGNED-%d", len(s.payloads)+len(s.failures)+1)
}

func (s *AppleSigner) marshal(value any) []byte {
	s.t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(s.t, err)
	return encoded
}

func buildTransactionInfo(notification AppleNotification) map[string]any {
	info := map[string]any{"originalTransactionId": notification.OriginalTransactionID}
	if notification.ExpiresAtMillis != nil {
		info["expiresDate"] = *notification.ExpiresAtMillis
	}
	return info
}
