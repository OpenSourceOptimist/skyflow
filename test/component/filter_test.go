package component

import (
	"context"
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
		filter           messages.Subscription
		expectingMessage bool
	}{
		{
			name:             "eventID filter misses",
			filter:           messages.Subscription{IDs: []event.ID{"randomEventID"}},
			expectingMessage: false,
		},
		{
			name:             "eventID filter matches",
			filter:           messages.Subscription{IDs: []event.ID{validEvent.ID}},
			expectingMessage: true,
		},
		{
			name:             "eventID filter misses",
			filter:           messages.Subscription{Authors: []event.PubKey{"randomPubkey"}},
			expectingMessage: false,
		},
		{
			name:             "eventID filter matches",
			filter:           messages.Subscription{Authors: []event.PubKey{validEvent.PubKey}},
			expectingMessage: true,
		},
		{
			name:             "kind filter misses",
			filter:           messages.Subscription{Kinds: []event.Kind{24343}},
			expectingMessage: false,
		},
		{
			name:             "kind filter matches",
			filter:           messages.Subscription{Kinds: []event.Kind{validEvent.Kind}},
			expectingMessage: true,
		},
		{
			name:             "since filter matches",
			filter:           messages.Subscription{Since: validEvent.CreatedAt - 1},
			expectingMessage: true,
		},
		{
			name:             "since filter does not matche",
			filter:           messages.Subscription{Since: validEvent.CreatedAt + 1},
			expectingMessage: false,
		},
		{
			name:             "until filter matches",
			filter:           messages.Subscription{Until: validEvent.CreatedAt + 1},
			expectingMessage: true,
		},
		{
			name:             "until filter does not matche",
			filter:           messages.Subscription{Until: validEvent.CreatedAt - 1},
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

			requestSub(ctx, t, conn, tc.filter)

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
	_, pub1 := NewKeyPair(t)
	_, pub2 := NewKeyPair(t)
	referencingPub1 := NewSignedEvent(t, EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub1.String(), ""}}})
	referencingPub2 := NewSignedEvent(t, EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub2.String(), ""}}})
	testcases := []struct {
		name            string
		allEvents       []nostr.Event
		filter          nostr.Filter
		recivedEventIDs []string
	}{
		{
			name:            "Enfoce limit, latest 3",
			allEvents:       []nostr.Event{createdAt100, createdAt200, createdAt300, createdAt400},
			filter:          nostr.Filter{Limit: 3},
			recivedEventIDs: []string{createdAt400.ID, createdAt300.ID, createdAt200.ID},
		},
		{
			name:            "Filter on pubkeys referenced in p tag with go-nostr library",
			allEvents:       []nostr.Event{referencingPub1, referencingPub2, createdAt100},
			filter:          nostr.Filter{Tags: nostr.TagMap{"p": []string{pub1.String()}}},
			recivedEventIDs: []string{referencingPub1.ID},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer clearMongo()
			ctx := context.Background()
			relay, err := nostr.RelayConnect(ctx, "ws://localhost:80")
			require.NoError(t, err, "relay connecting")
			defer relay.Close()
			for _, e := range tc.allEvents {
				require.Equal(t, "success", relay.Publish(ctx, e).String())
			}
			sub := relay.Subscribe(ctx, nostr.Filters{tc.filter})
			reciviedEvents := slice.ReadSlice(sub.Events, 500*time.Millisecond)
			require.NoError(t, err, "retriving events")
			recivedEventIDs := slice.Map(reciviedEvents, func(e *nostr.Event) string { return e.ID })
			require.Equal(t, tc.recivedEventIDs, recivedEventIDs)

		})
	}
}

func TestMoreComplicatedFiltering(t *testing.T) {
	createdAt100 := toEvent(NewSignedEvent(t, EventOptions{CreatedAt: time.Unix(100, 0)}))
	createdAt200 := toEvent(NewSignedEvent(t, EventOptions{CreatedAt: time.Unix(200, 0)}))
	createdAt300 := toEvent(NewSignedEvent(t, EventOptions{CreatedAt: time.Unix(300, 0)}))
	createdAt400 := toEvent(NewSignedEvent(t, EventOptions{CreatedAt: time.Unix(400, 0)}))
	_, pub1 := NewKeyPair(t)
	_, pub2 := NewKeyPair(t)
	referencingPub1 := toEvent(NewSignedEvent(t, EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub1.String(), ""}}}))
	referencingPub2 := toEvent(NewSignedEvent(t, EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub2.String(), ""}}}))
	testcases := []struct {
		name            string
		allEvents       []event.Event
		filter          []messages.Subscription
		recivedEventIDs []event.ID
	}{
		{
			name:            "Enfoce limit, sorted on created_at",
			allEvents:       []event.Event{createdAt100, createdAt200, createdAt300, createdAt400},
			filter:          []messages.Subscription{{Limit: 3}},
			recivedEventIDs: []event.ID{createdAt400.ID, createdAt300.ID, createdAt200.ID},
		},
		{
			name:            "Filter on pubkeys referenced in p tag",
			allEvents:       []event.Event{referencingPub1, referencingPub2, createdAt100},
			filter:          []messages.Subscription{{P: []event.PubKey{event.PubKey(pub1)}}},
			recivedEventIDs: []event.ID{referencingPub1.ID},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer clearMongo()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conn, closer := newSocket(ctx, t)
			defer closer()
			for _, e := range tc.allEvents {
				publish(ctx, t, e, conn)
			}
			sub := requestSub(ctx, t, conn, tc.filter...)
			reciviedEvents := slice.ReadSlice(
				listenForEventsOnSub(ctx, t, conn, sub),
				500*time.Millisecond,
			)
			recivedEventIDs := slice.Map(reciviedEvents, func(e event.Event) event.ID { return e.ID })
			require.Equal(t, tc.recivedEventIDs, recivedEventIDs)
		})
	}
}
