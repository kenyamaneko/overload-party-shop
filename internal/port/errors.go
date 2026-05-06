package port

import "errors"

// ErrNotFound は対象行が存在しない場合に返される。
var ErrNotFound = errors.New("not found")

// ErrUnsupportedPlatform は ios/android 以外の platform を受け取った場合に返される。
var ErrUnsupportedPlatform = errors.New("unsupported platform")
