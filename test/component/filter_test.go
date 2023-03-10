package component

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	nostr "github.com/nbd-wtf/go-nostr"
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

func TestFiltering(t *testing.T) {
	createdAt100 := NewSignedEvent(t, EventOptions{CreatedAt: time.Unix(100, 0)})
	createdAt200 := NewSignedEvent(t, EventOptions{CreatedAt: time.Unix(200, 0)})
	createdAt300 := NewSignedEvent(t, EventOptions{CreatedAt: time.Unix(300, 0)})
	createdAt400 := NewSignedEvent(t, EventOptions{CreatedAt: time.Unix(400, 0)})
	testcases := []struct {
		name            string
		allEvents       []nostr.Event
		filter          nostr.Filter
		recivedEventIDs []string
	}{
		{ // NIP01: When limit: n is present it is assumed that the events returned in the initial query will be the latest n events.
			name:            "Enfoce limit, sorted on created_at",
			allEvents:       []nostr.Event{createdAt100, createdAt200, createdAt300, createdAt400},
			filter:          nostr.Filter{Limit: 3},
			recivedEventIDs: []string{createdAt400.ID, createdAt300.ID, createdAt200.ID},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer clearMongo()
			fmt.Println(tc.name)
			ctx := context.Background()
			relay, err := nostr.RelayConnect(ctx, "ws://localhost:80")
			require.NoError(t, err, "relay connecting")
			defer relay.Close()
			for _, e := range tc.allEvents {
				require.Equal(t, "success", relay.Publish(ctx, e).String())
			}
			fmt.Println("subscribing")
			sub := relay.Subscribe(ctx, nostr.Filters{tc.filter})
			reciviedEvents := slice.ReadSlice(sub.Events, 500*time.Millisecond)
			require.NoError(t, err, "retriving events")
			recivedEventIDs := slice.Map(reciviedEvents, func(e *nostr.Event) string { return e.ID })
			require.Equal(t, tc.recivedEventIDs, recivedEventIDs)

		})
	}
}
