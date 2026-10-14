// Package logtest は既定のロガーに出たログを捕捉するテストヘルパ。
package logtest

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// Record は捕捉した 1 件のログ。
type Record struct {
	Level   slog.Level
	Message string
	Attrs   map[string]slog.Value
}

// String はログの項目 key の値を文字列で返す。
func (r Record) String(key string) string {
	return r.Attrs[key].String()
}

// Int はログの項目 key の値を整数で返す。
func (r Record) Int(key string) int64 {
	return r.Attrs[key].Int64()
}

// Duration はログの項目 key の値を時間で返す。
func (r Record) Duration(key string) time.Duration {
	return r.Attrs[key].Duration()
}

// Logs は既定のロガーに出たログの捕捉結果。
type Logs struct {
	mu      sync.Mutex
	records []Record
}

// Capture は既定のロガーを捕捉用に差し替え、テストの終了時に元へ戻す。
func Capture(t *testing.T) *Logs {
	t.Helper()
	logs := &Logs{}
	previous := slog.Default()
	slog.SetDefault(slog.New(&captureHandler{logs: logs}))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return logs
}

// Records は捕捉したすべてのログを出力順に返す。
func (l *Logs) Records() []Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Record(nil), l.records...)
}

// SelectByMessage はメッセージが message のログを出力順に返す。
func (l *Logs) SelectByMessage(message string) []Record {
	var matched []Record
	for _, record := range l.Records() {
		if record.Message == message {
			matched = append(matched, record)
		}
	}
	return matched
}

// ExcludeByMessage はメッセージが message でないログを出力順に返す。
func (l *Logs) ExcludeByMessage(message string) []Record {
	var matched []Record
	for _, record := range l.Records() {
		if record.Message != message {
			matched = append(matched, record)
		}
	}
	return matched
}

// SelectByLevel はレベルが level のログを出力順に返す。
func (l *Logs) SelectByLevel(level slog.Level) []Record {
	var matched []Record
	for _, record := range l.Records() {
		if record.Level == level {
			matched = append(matched, record)
		}
	}
	return matched
}

// SelectByMessageAndLevel はメッセージが message でレベルが level のログを出力順に返す。
func (l *Logs) SelectByMessageAndLevel(message string, level slog.Level) []Record {
	var matched []Record
	for _, record := range l.SelectByMessage(message) {
		if record.Level == level {
			matched = append(matched, record)
		}
	}
	return matched
}

func (l *Logs) append(record Record) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

type captureHandler struct {
	logs  *Logs
	attrs []slog.Attr
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *captureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]slog.Value)
	for _, attr := range h.attrs {
		attrs[attr.Key] = attr.Value
	}
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value
		return true
	})
	h.logs.append(Record{Level: record.Level, Message: record.Message, Attrs: attrs})
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &captureHandler{logs: h.logs, attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}

func (h *captureHandler) WithGroup(string) slog.Handler {
	return h
}
