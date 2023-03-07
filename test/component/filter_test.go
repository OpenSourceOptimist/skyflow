package component

import (
	"context"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/stretchr/testify/require"
)

func TestBasicFiltering(t *testing.T) {

	testcases := []struct {
		name             string
		filter           messages.RequestFilter
		expectingMessage bool
	}{
		{
			name:             "eventID filter misses",
			filter:           messages.RequestFilter{IDs: []event.EventID{"randomEventID"}},
			expectingMessage: false,
		},
		{
			name:             "eventID filter matches",
			filter:           messages.RequestFilter{IDs: []event.EventID{validEvent.ID}},
			expectingMessage: true,
		},
		{
			name:             "eventID filter misses",
			filter:           messages.RequestFilter{Authors: []event.PubKey{"randomPubkey"}},
			expectingMessage: false,
		},
		{
			name:             "eventID filter matches",
			filter:           messages.RequestFilter{Authors: []event.PubKey{validEvent.PubKey}},
			expectingMessage: true,
		},
		{
			name:             "kind filter misses",
			filter:           messages.RequestFilter{Kinds: []event.EventKind{24343}},
			expectingMessage: false,
		},
		{
			name:             "kind filter matches",
			filter:           messages.RequestFilter{Kinds: []event.EventKind{validEvent.Kind}},
			expectingMessage: true,
		},
		{
			name:             "since filter matches",
			filter:           messages.RequestFilter{Since: validEvent.CreatedAt - 1},
			expectingMessage: true,
		},
		{
			name:             "since filter does not matche",
			filter:           messages.RequestFilter{Since: validEvent.CreatedAt + 1},
			expectingMessage: false,
		},
		{
			name:             "until filter matches",
			filter:           messages.RequestFilter{Until: validEvent.CreatedAt + 1},
			expectingMessage: true,
		},
		{
			name:             "until filter does not matche",
			filter:           messages.RequestFilter{Until: validEvent.CreatedAt - 1},
			expectingMessage: false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer clearMongo()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			conn, closeFunc := newSocket(ctx, t)
			defer closeFunc()

			publish(ctx, t, validEvent, conn)

			requestSub(ctx, t, tc.filter, conn)

			ctx, cancel = context.WithTimeout(ctx, time.Second)
			defer cancel()
			_, _, err := conn.Read(ctx)
			if tc.expectingMessage {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
		})
	}
}
