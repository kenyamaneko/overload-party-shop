package port

import "errors"

// ErrNotFound は対象行が存在しない場合に返される。
var ErrNotFound = errors.New("not found")

// ErrUnsupportedPlatform は repo メソッドが ios/android 以外の platform を
// 受け取った場合に返される。token テーブルが platform ごとに分かれているため
// repo 層で platform 制約を表明する。
var ErrUnsupportedPlatform = errors.New("unsupported platform")
