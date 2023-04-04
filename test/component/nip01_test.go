package component

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/OpenSourceOptimist/skyflow/test/help"
	"github.com/google/uuid"
	nostr "github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

func TestNIP01BasicFlow(t *testing.T) {
	defer clearMongo()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, "ws://localhost:80", nil)
	require.NoError(t, err, "websocket dial error")
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "handshake status")
	defer conn.Close(websocket.StatusGoingAway, "bye")

	testEvent := event.Event{
		ID:        event.ID("d7dd5eb3ab747e16f8d0212d53032ea2a7cadef53837e5a6c66d42849fcb9027"),
		Kind:      event.Kind(1),
		PubKey:    event.PubKey("22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c"),
		CreatedAt: event.Timestamp(1670869179),
		Content:   "NOSTR \"WINE-ACCOUNT\" WITH HARVEST DATE STAMPED\n\n\n\"The older the wine, the greater its reputation\"\n\n\n22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c\n\n\nNWA 2022-12-12\nAA",
		Tags:      []event.Tag{{"client", "astral"}},
		Sig:       event.Signature("f110e4fdf67835fb07abc72469933c40bdc7334615610cade9554bf00945a1cebf84f8d079ec325d26fefd76fe51cb589bdbe208ac9cdbd63351ddad24a57559"),
	}

	reqBytes, err := json.Marshal([]interface{}{"EVENT", testEvent})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))

	subscriptionID := uuid.NewString()
	reqBytes, err = json.Marshal([]interface{}{"REQ", subscriptionID, messages.Filter{IDs: []event.ID{testEvent.ID}}})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))

	msgType, responseBytes, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, msgType)

	var eventDataMsg []json.RawMessage
	require.NoError(t, json.Unmarshal(responseBytes, &eventDataMsg))
	require.Len(t, eventDataMsg, 3)
	var recivedMsgType string
	require.NoError(t, json.Unmarshal(eventDataMsg[0], &recivedMsgType))
	require.Equal(t, "EVENT", recivedMsgType)
}

var validEvent = event.Event{
	ID:        event.ID("d7dd5eb3ab747e16f8d0212d53032ea2a7cadef53837e5a6c66d42849fcb9027"),
	Kind:      event.Kind(1),
	PubKey:    event.PubKey("22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c"),
	CreatedAt: event.Timestamp(1670869179),
	Content:   "NOSTR \"WINE-ACCOUNT\" WITH HARVEST DATE STAMPED\n\n\n\"The older the wine, the greater its reputation\"\n\n\n22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c\n\n\nNWA 2022-12-12\nAA",
	Tags:      []event.Tag{{"client", "astral"}},
	Sig:       event.Signature("f110e4fdf67835fb07abc72469933c40bdc7334615610cade9554bf00945a1cebf84f8d079ec325d26fefd76fe51cb589bdbe208ac9cdbd63351ddad24a57559"),
}

func TestNIP01Closing(t *testing.T) {
	defer clearMongo()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, closer := help.NewSocket(ctx, t)
	defer closer()
	subID1 := help.RequestSub(ctx, t, conn, messages.Filter{})
	help.CancelSub(ctx, t, subID1, conn)
	help.Publish(ctx, t, validEvent, conn)
	subID2 := help.RequestSub(ctx, t, conn, messages.Filter{})
	sub, e, err := help.ReadEvent(ctx, t, conn)
	require.NoError(t, err)
	require.Equal(t, subID2, sub)
	require.Equal(t, validEvent.ID, e.ID)
}

func TestNIP01BasicFiltering(t *testing.T) {
	testcases := []struct {
		name             string
		filter           messages.Filter
		expectingMessage bool
	}{
		{
			name:             "eventID filter misses",
			filter:           messages.Filter{IDs: []event.ID{"randomEventID"}},
			expectingMessage: false,
		},
		{
			name:             "eventID filter matches",
			filter:           messages.Filter{IDs: []event.ID{validEvent.ID}},
			expectingMessage: true,
		},
		{
			name:             "eventID prefix",
			filter:           messages.Filter{IDs: []event.ID{validEvent.ID[:5]}},
			expectingMessage: true,
		},
		{
			name:             "authors filter misses",
			filter:           messages.Filter{Authors: []event.PubKey{"randomPubkey"}},
			expectingMessage: false,
		},
		{
			name:             "authors filter matches",
			filter:           messages.Filter{Authors: []event.PubKey{validEvent.PubKey}},
			expectingMessage: true,
		},
		{
			name:             "authors prefix",
			filter:           messages.Filter{Authors: []event.PubKey{validEvent.PubKey[:5]}},
			expectingMessage: true,
		},
		{
			name:             "kind filter misses",
			filter:           messages.Filter{Kinds: []event.Kind{24343}},
			expectingMessage: false,
		},
		{
			name:             "kind filter matches",
			filter:           messages.Filter{Kinds: []event.Kind{validEvent.Kind}},
			expectingMessage: true,
		},
		{
			name:             "since filter matches",
			filter:           messages.Filter{Since: validEvent.CreatedAt - 1},
			expectingMessage: true,
		},
		{
			name:             "since filter does not matche",
			filter:           messages.Filter{Since: validEvent.CreatedAt + 1},
			expectingMessage: false,
		},
		{
			name:             "until filter matches",
			filter:           messages.Filter{Until: validEvent.CreatedAt + 1},
			expectingMessage: true,
		},
		{
			name:             "until filter does not matche",
			filter:           messages.Filter{Until: validEvent.CreatedAt - 1},
			expectingMessage: false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer clearMongo()
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			conn, closeFunc := help.NewSocket(ctx, t)
			defer closeFunc()

			help.Publish(ctx, t, validEvent, conn)

			help.RequestSub(ctx, t, conn, tc.filter)

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

func TestNIP01Filtering(t *testing.T) {
	createdAt100 := help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(100, 0)})
	createdAt200 := help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(200, 0)})
	createdAt300 := help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(300, 0)})
	createdAt400 := help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(400, 0)})
	_, pub1 := help.NewKeyPair(t)
	_, pub2 := help.NewKeyPair(t)
	referencingPub1 := help.NewSignedEvent(t, help.EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub1.String(), ""}}})
	referencingPub2 := help.NewSignedEvent(t, help.EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub2.String(), ""}}})
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

func TestNIP01MoreComplicatedFiltering(t *testing.T) {
	createdAt100 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(100, 0)}))
	createdAt200 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(200, 0)}))
	createdAt300 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(300, 0)}))
	createdAt400 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(400, 0)}))
	_, pub1 := help.NewKeyPair(t)
	_, pub2 := help.NewKeyPair(t)
	referencingPub1 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub1.String(), ""}}}))
	referencingPub2 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub2.String(), ""}}}))
	testcases := []struct {
		name            string
		allEvents       []event.Event
		filter          []messages.Filter
		recivedEventIDs []event.ID
		requireOrder    bool
	}{
		{
			name:            "Enfoce limit, sorted on created_at",
			allEvents:       []event.Event{createdAt100, createdAt200, createdAt300, createdAt400},
			filter:          []messages.Filter{{Limit: 3}},
			recivedEventIDs: []event.ID{createdAt400.ID, createdAt300.ID, createdAt200.ID},
			requireOrder:    true,
		},
		{
			name:            "Filter on pubkeys referenced in p tag",
			allEvents:       []event.Event{referencingPub1, referencingPub2, createdAt100},
			filter:          []messages.Filter{{P: []event.PubKey{event.PubKey(pub1)}}},
			recivedEventIDs: []event.ID{referencingPub1.ID},
			requireOrder:    true,
		},
		{
			name:      "OR",
			allEvents: []event.Event{referencingPub1, referencingPub2, createdAt100, createdAt200},
			filter: []messages.Filter{
				{P: []event.PubKey{event.PubKey(pub1)}},
				{IDs: []event.ID{createdAt100.ID}},
			},
			recivedEventIDs: []event.ID{createdAt100.ID, referencingPub1.ID},
			requireOrder:    false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer clearMongo()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conn, closer := help.NewSocket(ctx, t)
			defer closer()
			for _, e := range tc.allEvents {
				help.Publish(ctx, t, e, conn)
			}
			sub := help.RequestSub(ctx, t, conn, tc.filter...)
			reciviedEvents := slice.ReadSlice(
				help.ListenForEventsOnSub(ctx, t, conn, sub),
				500*time.Millisecond,
			)
			recivedEventIDs := slice.Map(reciviedEvents, func(e event.Event) event.ID { return e.ID })
			if tc.requireOrder {
				require.Equal(t, tc.recivedEventIDs, recivedEventIDs)
			} else {
				require.ElementsMatch(t, tc.recivedEventIDs, recivedEventIDs)
			}
		})
	}
}

func TestNIP01GetEventsAfterInitialSync(t *testing.T) {
	defer clearMongo()
	priv, pub := help.NewKeyPair(t)
	conn1 := help.NewConnection(t)
	ctx := context.Background()
	initialPublishedEvent := help.NewSignedEvent(t, help.EventOptions{PrivKey: priv, Content: "hello world"})
	status := conn1.Publish(ctx, initialPublishedEvent)
	require.Equal(t, nostr.PublishStatusSucceeded, status)

	conn2 := help.NewConnection(t)
	subscription := conn2.Subscribe(ctx, nostr.Filters{{Authors: []string{string(pub)}}})
	select {
	case initialRecivedEvent := <-subscription.Events:
		require.Equal(t, help.ToEvent(initialPublishedEvent), help.ToEvent(*initialRecivedEvent))
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for initial event to be recived")
	}

	secondPublishedEvent := help.NewSignedEvent(t, help.EventOptions{PrivKey: priv, Content: "hello again"})
	require.Equal(t,
		conn1.Publish(ctx, secondPublishedEvent),
		nostr.PublishStatusSucceeded,
	)

	select {
	case secondRecivedEvent := <-subscription.Events:
		require.Equal(t, help.ToEvent(secondPublishedEvent), help.ToEvent(*secondRecivedEvent))
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for second event to be recived")
	}
}

func TestNIP01VerificationEventID(t *testing.T) {
	defer clearMongo()
	ctx := context.Background()
	conn, _, _ := websocket.Dial(ctx, "ws://localhost:80", nil)
	defer conn.Close(websocket.StatusGoingAway, "bye")
	testEvent := event.Event{
		ID:        event.ID("d8dd5eb23b747e16f8d0212d53032ea2a7cadef53837e5a6c66142843fcb9027"), // modified in a couple of places
		Kind:      event.Kind(1),
		PubKey:    event.PubKey("22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c"),
		CreatedAt: event.Timestamp(1670869179),
		Content:   "NOSTR \"WINE-ACCOUNT\" WITH HARVEST DATE STAMPED\n\n\n\"The older the wine, the greater its reputation\"\n\n\n22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c\n\n\nNWA 2022-12-12\nAA",
		Tags:      []event.Tag{{"client", "astral"}},
		Sig:       event.Signature("f110e4fdf67835fb07abc72469933c40bdc7334615610cade9554bf00945a1cebf84f8d079ec325d26fefd76fe51cb589bdbe208ac9cdbd63351ddad24a57559"),
	}

	// Writing a bad event should work
	eventMsg, _ := json.Marshal([]any{"EVENT", testEvent})

	require.NoError(t, conn.Write(ctx, websocket.MessageText, eventMsg))
	// But should not show up in a request
	reqBytes, _ := json.Marshal([]interface{}{"REQ", uuid.NewString(), messages.Filter{IDs: []event.ID{testEvent.ID}}})
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))
	ctx, cancelFunc := context.WithTimeout(ctx, time.Second)
	defer cancelFunc()
	_, readBytes, err := conn.Read(ctx)
	require.Equal(t, "", string(readBytes))
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNIP01VerificationSignature(t *testing.T) {
	defer clearMongo()
	ctx := context.Background()
	conn, _, _ := websocket.Dial(ctx, "ws://localhost:80", nil)
	defer conn.Close(websocket.StatusGoingAway, "bye")
	testEvent := event.Event{
		ID:        event.ID("d7dd5eb3ab747e16f8d0212d53032ea2a7cadef53837e5a6c66d42849fcb9027"),
		Kind:      event.Kind(1),
		PubKey:    event.PubKey("22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c"),
		CreatedAt: event.Timestamp(1670869179),
		Content:   "NOSTR \"WINE-ACCOUNT\" WITH HARVEST DATE STAMPED\n\n\n\"The older the wine, the greater its reputation\"\n\n\n22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c\n\n\nNWA 2022-12-12\nAA",
		Tags:      []event.Tag{{"client", "astral"}},
		Sig:       event.Signature("f111e4fdf67835fb07abc72469933c40bdc7334615610cade9554bf00945a1cebf84f8d079ec325d26fefd76fe51cb589bdbe208ac9cdbd63351ddad24a57559"), // modified
	}

	// Writing a bad event should work
	eventMsg, _ := json.Marshal([]any{"EVENT", testEvent})
	require.NoError(t, conn.Write(ctx, websocket.MessageText, eventMsg))
	// But should not show up in a request
	reqBytes, _ := json.Marshal([]interface{}{"REQ", uuid.NewString(), messages.Filter{IDs: []event.ID{testEvent.ID}}})
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))
	ctx, cancelFunc := context.WithTimeout(ctx, time.Second)
	defer cancelFunc()
	_, readBytes, err := conn.Read(ctx)
	require.Equal(t, "", string(readBytes))
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNIP01DuplicateSubscriptionIDsBetweenSessions(t *testing.T) {
	defer clearMongo()
	ctx := context.Background()
	subID := uuid.NewString()
	bytes, err := json.Marshal([]interface{}{"REQ", subID, messages.Filter{}})
	require.NoError(t, err)

	conn1, closer1 := help.NewSocket(ctx, t)
	defer closer1()
	require.NoError(t, conn1.Write(ctx, websocket.MessageText, bytes))

	conn2, closer2 := help.NewSocket(ctx, t)
	defer closer2()
	require.NoError(t, conn2.Write(ctx, websocket.MessageText, bytes))

	conn3, closer3 := help.NewSocket(ctx, t)
	defer closer3()
	content := uuid.NewString()
	help.Publish(ctx,
		t,
		help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Content: content})),
		conn3)

	select {
	case e := <-help.ListenForEventsOnSub(ctx, t, conn1, messages.SubscriptionID(subID)):
		require.Equal(t, content, e.Content)
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for event")
	}
	select {
	case e := <-help.ListenForEventsOnSub(ctx, t, conn2, messages.SubscriptionID(subID)):
		require.Equal(t, content, e.Content)
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for event")
	}
}

func TestNIP01DuplicateSubscriptionIDsBetweenSessionsClosing(t *testing.T) {
	defer clearMongo()
	ctx := context.Background()
	subID := messages.SubscriptionID(uuid.NewString())
	bytes, err := json.Marshal([]interface{}{"REQ", subID, messages.Filter{}})
	require.NoError(t, err)

	conn1, closer1 := help.NewSocket(ctx, t)
	defer closer1()
	require.NoError(t, conn1.Write(ctx, websocket.MessageText, bytes))

	conn2, closer2 := help.NewSocket(ctx, t)
	defer closer2()
	require.NoError(t, conn2.Write(ctx, websocket.MessageText, bytes))

	help.CancelSub(ctx, t, subID, conn2)

	conn3, closer3 := help.NewSocket(ctx, t)
	defer closer3()
	content := uuid.NewString()
	help.Publish(ctx,
		t,
		help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Content: content})),
		conn3)

	select {
	case e := <-help.ListenForEventsOnSub(ctx, t, conn1, subID):
		require.Equal(t, content, e.Content)
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for event")
	}
}
