package port

import "errors"

// ErrNotFound は対象行が存在しない場合に返される。
var ErrNotFound = errors.New("not found")
