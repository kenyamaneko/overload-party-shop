package apishopfake

import (
	"crypto/rand"
	"fmt"
)

// newEventID は RFC 4122 v4 相当の UUID 文字列を返す。本パッケージは api-shop
// module に属し google/uuid を依存に入れたくないため、crypto/rand + stdlib で
// 自前生成する (テスト用 fake 専用のため外部ライブラリを増やさない方針)。
func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read は実装上エラーを返さない (darwin/linux とも
		// getentropy / getrandom が常に成功)。理論上の保険として panic する。
		panic(fmt.Sprintf("apishopfake: crypto/rand.Read failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // RFC 4122 version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
