package purchasetest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const outboxEventSelect = `
	SELECT event_id::text, event_type, payload
	FROM shop.outbox_events
`

// OutboxEvent はアウトボックスに記録された1件のイベント。Payload はイベント本文を JSON として読んだもの。
type OutboxEvent struct {
	EventID   string
	EventType string
	Payload   map[string]any
}

// TimeField はイベント本文の key の項目を RFC 3339 形式の日時として読む。
func (e OutboxEvent) TimeField(t *testing.T, key string) time.Time {
	t.Helper()
	value, isPresent := e.Payload[key]
	require.True(t, isPresent, "イベント本文に %s が無い", key)
	text, isText := value.(string)
	require.True(t, isText, "イベント本文の %s が文字列でない", key)
	parsed, err := time.Parse(time.RFC3339, text)
	require.NoError(t, err)
	return parsed
}

// OutboxSnapshot は取得した時点でアウトボックスに記録済みだったイベントを控え、それ以降に加わったイベントだけを読み出せるようにする。
type OutboxSnapshot struct {
	pool          *pgxpool.Pool
	knownEventIDs []string
}

// TakeOutboxSnapshot は現時点でアウトボックスに記録済みのイベントを控える。
func TakeOutboxSnapshot(t *testing.T, pool *pgxpool.Pool) *OutboxSnapshot {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT event_id::text FROM shop.outbox_events`)
	require.NoError(t, err)
	known := collectRows(t, rows, func(row pgx.Row) (string, error) {
		var eventID string
		err := row.Scan(&eventID)
		return eventID, err
	})
	return &OutboxSnapshot{pool: pool, knownEventIDs: append([]string{}, known...)}
}

// AllNewEvents は控えた時点のあとにアウトボックスへ加わったイベントを、種別を問わず返す。
func (s *OutboxSnapshot) AllNewEvents(t *testing.T) []OutboxEvent {
	t.Helper()
	return s.queryNewEvents(t, outboxEventSelect+`
		WHERE event_id::text <> ALL($1)
		ORDER BY created_at, event_id
	`, s.knownEventIDs)
}

// NewEvents は控えた時点のあとにアウトボックスへ加わったイベントのうち、種別が eventType のものを返す。
func (s *OutboxSnapshot) NewEvents(t *testing.T, eventType string) []OutboxEvent {
	t.Helper()
	return s.queryNewEvents(t, outboxEventSelect+`
		WHERE event_id::text <> ALL($1) AND event_type = $2
		ORDER BY created_at, event_id
	`, s.knownEventIDs, eventType)
}

func (s *OutboxSnapshot) queryNewEvents(t *testing.T, query string, args ...any) []OutboxEvent {
	t.Helper()
	rows, err := s.pool.Query(t.Context(), query, args...)
	require.NoError(t, err)
	return collectRows(t, rows, func(row pgx.Row) (OutboxEvent, error) {
		var event OutboxEvent
		var payload []byte
		if err := row.Scan(&event.EventID, &event.EventType, &payload); err != nil {
			return event, err
		}
		return event, json.Unmarshal(payload, &event.Payload)
	})
}
