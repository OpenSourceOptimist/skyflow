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

func TestNIP01Filters(t *testing.T) {
	createdAt100 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(100, 0)}))
	createdAt200 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(200, 0)}))
	createdAt300 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(300, 0)}))
	createdAt400 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(400, 0)}))
	priv1, pub1 := help.NewKeyPair(t)
	priv2, pub2 := help.NewKeyPair(t)
	priv3, _ := help.NewKeyPair(t)
	referencingPub1 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub1.String(), ""}}}))
	referencingPub2 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Tags: nostr.Tags{nostr.Tag{"p", pub2.String(), ""}}}))
	fromPub1 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{PrivKey: priv1}))
	fromPub2 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{PrivKey: priv2}))
	fromPub3 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{PrivKey: priv3}))
	kind1 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Kind: 1}))
	kind2 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Kind: 2}))
	kind3 := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Kind: 3}))
	oldest := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(0, 0)}))
	newer := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(1000, 0)}))
	newest := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(10000, 0)}))

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
		{
			name:            "eventID",
			allEvents:       []event.Event{createdAt200, createdAt100, createdAt300},
			filter:          []messages.Filter{{IDs: []event.ID{createdAt100.ID}}},
			recivedEventIDs: []event.ID{createdAt100.ID},
		},
		{
			name:            "eventID prefix",
			allEvents:       []event.Event{createdAt200, createdAt100, createdAt300},
			filter:          []messages.Filter{{IDs: []event.ID{createdAt100.ID[:5]}}},
			recivedEventIDs: []event.ID{createdAt100.ID},
		},
		{
			name:            "authors",
			allEvents:       []event.Event{fromPub1, fromPub2, fromPub3},
			filter:          []messages.Filter{{Authors: []event.PubKey{fromPub2.PubKey}}},
			recivedEventIDs: []event.ID{fromPub2.ID},
		},
		{
			name:            "authors prefix",
			allEvents:       []event.Event{fromPub1, fromPub2, fromPub3},
			filter:          []messages.Filter{{Authors: []event.PubKey{fromPub2.PubKey[:5]}}},
			recivedEventIDs: []event.ID{fromPub2.ID},
		},
		{
			name:            "kind",
			allEvents:       []event.Event{kind1, kind2, kind3},
			filter:          []messages.Filter{{Kinds: []event.Kind{kind2.Kind}}},
			recivedEventIDs: []event.ID{kind2.ID},
		},
		{
			name:            "since",
			allEvents:       []event.Event{oldest, newer, newest},
			filter:          []messages.Filter{{Since: newer.CreatedAt}},
			recivedEventIDs: []event.ID{newest.ID},
		},
		{
			name:            "until",
			allEvents:       []event.Event{oldest, newer, newest},
			filter:          []messages.Filter{{Until: newer.CreatedAt}},
			recivedEventIDs: []event.ID{oldest.ID},
		},
		{
			name:            "since-until",
			allEvents:       []event.Event{oldest, newer, newest},
			filter:          []messages.Filter{{Since: oldest.CreatedAt, Until: newest.CreatedAt}},
			recivedEventIDs: []event.ID{newer.ID},
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
				len(tc.recivedEventIDs),
				time.Second,
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
	conn, closer := help.NewSocket(ctx, t)
	defer closer()

	badEvent := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(1000, 0)}))
	badEvent.ID = "really wrong event id"
	help.Publish(ctx, t, badEvent, conn)
	olderevent := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(0, 0)}))
	help.Publish(ctx, t, olderevent, conn)

	subID := help.RequestSub(ctx, t, conn)
	found := slice.ReadSlice(help.ListenForEventsOnSub(ctx, t, conn, subID), 1, time.Second)
	require.Equal(t, []event.ID{olderevent.ID}, slice.Map(found, func(e event.Event) event.ID { return e.ID }))
}

func TestNIP01VerificationSignature(t *testing.T) {
	defer clearMongo()
	ctx := context.Background()
	conn, closer := help.NewSocket(ctx, t)
	defer closer()

	badEvent := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(1000, 0)}))
	badEvent.Sig = "really wrong signature"
	help.Publish(ctx, t, badEvent, conn)
	olderevent := help.ToEvent(help.NewSignedEvent(t, help.EventOptions{CreatedAt: time.Unix(0, 0)}))
	help.Publish(ctx, t, olderevent, conn)

	subID := help.RequestSub(ctx, t, conn)
	found := slice.ReadSlice(help.ListenForEventsOnSub(ctx, t, conn, subID), 1, time.Second)
	require.Equal(t, []event.ID{olderevent.ID}, slice.Map(found, func(e event.Event) event.ID { return e.ID }))
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
