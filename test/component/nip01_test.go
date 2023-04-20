package component

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/OpenSourceOptimist/skyflow/test/help"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

func TestNIP01BasicFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, read, closer := help.NewSocket(ctx, t)
	defer closer()

	testEvent := help.Event(t, help.EventOptions{})
	eventBytes, err := json.Marshal([]interface{}{"EVENT", testEvent})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, eventBytes))

	subscriptionID := uuid.NewString()
	reqBytes, err := json.Marshal([]interface{}{"REQ", subscriptionID, messages.Filter{IDs: []event.ID{testEvent.ID}}})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))

	timeout := time.After(time.Second)
	for responseBytes := range read {
		var eventDataMsg []json.RawMessage
		require.NoError(t, json.Unmarshal(responseBytes, &eventDataMsg))
		var recivedMsgType string
		require.NoError(t, json.Unmarshal(eventDataMsg[0], &recivedMsgType))
		if recivedMsgType == "EVENT" {
			require.Len(t, eventDataMsg, 3)
			return
		}
		select {
		case <-timeout:
			require.FailNow(t, "timed out waiting for event")
		default:
		}
	}
}

func TestNIP01Closing(t *testing.T) {
	validEvent := help.Event(t, help.EventOptions{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, read, closer := help.NewSocket(ctx, t)
	defer closer()
	subID1, sub1Closer := help.RequestSub(ctx, t, conn, messages.Filter{IDs: []event.ID{validEvent.ID}})
	defer sub1Closer()
	help.CloseSubscription(ctx, t, subID1, conn)
	help.Publish(ctx, t, validEvent, conn)
	subID2, sub2Closer := help.RequestSub(ctx, t, conn, messages.Filter{IDs: []event.ID{validEvent.ID}})
	defer sub2Closer()
	e, found := help.ReadEvent(ctx, t, read, subID2, time.Second)
	require.True(t, found)
	require.Equal(t, e.ID, e.ID)
}

func TestNIP01Filters(t *testing.T) {
	priv1, pub1 := help.NewKeyPair(t)
	priv2, pub2 := help.NewKeyPair(t)
	priv3, _ := help.NewKeyPair(t)
	testcases := []struct {
		name                   string
		allEvents              []event.Event
		filter                 []messages.Filter
		recivedEventsAtIndices []int
		requireOrder           bool
	}{
		{
			name: "Enfoce limit, sorted on created_at",
			allEvents: []event.Event{
				help.EventWithCreatedAt(t, time.Unix(100, 0)),
				help.EventWithCreatedAt(t, time.Unix(200, 0)),
				help.EventWithCreatedAt(t, time.Unix(300, 0)),
				help.EventWithCreatedAt(t, time.Unix(400, 0)),
			},
			filter:                 []messages.Filter{{Limit: 3}},
			recivedEventsAtIndices: []int{3, 2, 1},
			requireOrder:           true,
		},
		{
			name: "Filter on pubkeys referenced in p tag",
			allEvents: []event.Event{
				help.EventWithPTagReferenceTo(t, pub1),
				help.EventWithPTagReferenceTo(t, pub2),
				help.EventWithCreatedAt(t, time.Unix(100, 0)),
			},
			filter:                 []messages.Filter{{P: []event.PubKey{event.PubKey(pub1)}}},
			recivedEventsAtIndices: []int{0},
			requireOrder:           true,
		},
		{
			name: "OR",
			allEvents: []event.Event{
				help.EventWithPTagReferenceTo(t, pub1),
				help.EventWithPTagReferenceTo(t, pub2),
				help.EventWithKind(t, 2),
				help.EventWithKind(t, 3),
			},
			filter: []messages.Filter{
				{P: []event.PubKey{event.PubKey(pub1)}},
				{Kinds: []event.Kind{2}},
			},
			recivedEventsAtIndices: []int{2, 0},
			requireOrder:           false,
		},
		{
			name: "authors",
			allEvents: []event.Event{
				help.EventWithPrivKey(t, priv1),
				help.EventWithPrivKey(t, priv2),
				help.EventWithPrivKey(t, priv3),
			},
			filter:                 []messages.Filter{{Authors: []event.PubKey{pub2}}},
			recivedEventsAtIndices: []int{1},
		},
		{
			name: "authors prefix",
			allEvents: []event.Event{
				help.EventWithPrivKey(t, priv1),
				help.EventWithPrivKey(t, priv2),
				help.EventWithPrivKey(t, priv3),
			},
			filter:                 []messages.Filter{{Authors: []event.PubKey{pub2[:5]}}},
			recivedEventsAtIndices: []int{1},
		},
		{
			name: "kind",
			allEvents: []event.Event{
				help.EventWithKind(t, 1),
				help.EventWithKind(t, 2),
				help.EventWithKind(t, 3),
			},
			filter:                 []messages.Filter{{Kinds: []event.Kind{2}}},
			recivedEventsAtIndices: []int{1},
		},
		{
			name: "since",
			allEvents: []event.Event{
				help.EventWithCreatedAt(t, time.Unix(100, 0)),
				help.EventWithCreatedAt(t, time.Unix(200, 0)),
				help.EventWithCreatedAt(t, time.Unix(300, 0)),
			},
			filter:                 []messages.Filter{{Since: event.Timestamp(201)}},
			recivedEventsAtIndices: []int{2},
		},
		{
			name: "until",
			allEvents: []event.Event{
				help.EventWithCreatedAt(t, time.Unix(100, 0)),
				help.EventWithCreatedAt(t, time.Unix(200, 0)),
				help.EventWithCreatedAt(t, time.Unix(300, 0)),
			},
			filter:                 []messages.Filter{{Until: event.Timestamp(199)}},
			recivedEventsAtIndices: []int{0},
		},
		{
			name: "since-until",
			allEvents: []event.Event{
				help.EventWithCreatedAt(t, time.Unix(100, 0)),
				help.EventWithCreatedAt(t, time.Unix(200, 0)),
				help.EventWithCreatedAt(t, time.Unix(300, 0)),
			},
			filter:                 []messages.Filter{{Since: 101, Until: 299}},
			recivedEventsAtIndices: []int{1},
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conn, read, closer := help.NewSocket(ctx, t)
			defer closer()
			for _, e := range tc.allEvents {
				help.Publish(ctx, t, e, conn)
				_, found := help.GetEvent(ctx, t, e.ID, time.Second)
				require.True(t, found, "event in setup not found after publishing it")
			}
			expectedRecivedEventsIDs := make([]event.ID, len(tc.recivedEventsAtIndices))
			for i, index := range tc.recivedEventsAtIndices {
				expectedRecivedEventsIDs[i] = tc.allEvents[index].ID
			}
			allIDs := slice.Map(tc.allEvents, func(e event.Event) event.ID { return e.ID })
			modifiedFilters := make([]messages.Filter, len(tc.filter))
			for i, filter := range tc.filter {
				if len(filter.IDs) != 0 {
					require.FailNow(t, "testing filtering in IDs not supported")
				}
				filter.IDs = allIDs
				modifiedFilters[i] = filter
			}

			sub, subCloser := help.RequestSub(ctx, t, conn, modifiedFilters...)
			defer subCloser()
			reciviedEvents := slice.ReadSlice(
				help.ListenForEventsOnSub(ctx, t, read, sub, time.Second),
				len(expectedRecivedEventsIDs),
				time.Second,
			)

			recivedEventIDs := slice.Map(reciviedEvents, func(e event.Event) event.ID { return e.ID })
			if tc.requireOrder {
				require.Equal(t, expectedRecivedEventsIDs, recivedEventIDs)
			} else {
				require.ElementsMatch(t, expectedRecivedEventsIDs, recivedEventIDs)
			}
		})
	}
}

func TestNIP01GetEventsAfterInitialSync(t *testing.T) {
	priv, pub := help.NewKeyPair(t)
	ctx := context.Background()
	conn1, _, closer1 := help.NewSocket(ctx, t)
	defer closer1()
	initialPublishedEvent := help.Event(t, help.EventOptions{PrivKey: priv, Content: "hello world"})
	help.Publish(ctx, t, initialPublishedEvent, conn1)
	_, found := help.GetEvent(ctx, t, initialPublishedEvent.ID, time.Second)
	require.True(t, found)

	conn2, read2, closer2 := help.NewSocket(ctx, t)
	defer closer2()
	subID, subCloser := help.RequestSub(ctx, t, conn2, messages.Filter{Authors: []event.PubKey{pub}})
	defer subCloser()
	initialRecivedEvent, found := help.ReadEvent(ctx, t, read2, subID, time.Second)
	require.True(t, found)
	require.Equal(t, initialPublishedEvent, initialRecivedEvent)

	secondPublishedEvent := help.Event(t, help.EventOptions{PrivKey: priv, Content: "hello again"})
	help.Publish(ctx, t, secondPublishedEvent, conn1)
	_, found = help.GetEvent(ctx, t, secondPublishedEvent.ID, time.Second)
	require.True(t, found)

	secondRecivedEvent, found := help.ReadEvent(ctx, t, read2, subID, time.Second)
	require.True(t, found)
	require.Equal(t, secondPublishedEvent, secondRecivedEvent)
}

func TestNIP01VerificationEventID(t *testing.T) {
	ctx := context.Background()
	conn, read, closer := help.NewSocket(ctx, t)
	defer closer()

	badEvent := help.Event(t, help.EventOptions{CreatedAt: time.Unix(1000, 0)})
	badEvent.ID = "really wrong event id"
	help.Publish(ctx, t, badEvent, conn)
	olderevent := help.Event(t, help.EventOptions{CreatedAt: time.Unix(0, 0)})
	help.Publish(ctx, t, olderevent, conn)

	subID, subCloser := help.RequestSub(ctx, t, conn, messages.Filter{IDs: []event.ID{badEvent.ID, olderevent.ID}})
	defer subCloser()
	foundEvent, found := help.ReadEvent(ctx, t, read, subID, time.Second)
	require.True(t, found)
	require.Equal(t, olderevent.ID, foundEvent.ID)
}

func TestNIP01VerificationSignature(t *testing.T) {
	ctx := context.Background()
	conn, read, closer := help.NewSocket(ctx, t)
	defer closer()

	badEvent := help.Event(t, help.EventOptions{CreatedAt: time.Unix(1000, 0)})
	badEvent.Sig = "really wrong signature"
	help.Publish(ctx, t, badEvent, conn)
	olderevent := help.Event(t, help.EventOptions{CreatedAt: time.Unix(0, 0)})
	help.Publish(ctx, t, olderevent, conn)

	subID, subCloser := help.RequestSub(ctx, t, conn, messages.Filter{IDs: []event.ID{badEvent.ID, olderevent.ID}})
	defer subCloser()
	foundEvent, found := help.ReadEvent(ctx, t, read, subID, time.Second)
	require.True(t, found)
	require.Equal(t, olderevent.ID, foundEvent.ID)
}

func TestNIP01DuplicateSubscriptionIDsBetweenSessions(t *testing.T) {
	ctx := context.Background()
	subID := uuid.NewString()
	eventSent := help.Event(t)
	bytes, err := json.Marshal([]interface{}{"REQ", subID, messages.Filter{IDs: []event.ID{eventSent.ID}}})
	require.NoError(t, err)

	conn1, read1, closer1 := help.NewSocket(ctx, t)
	defer closer1()
	require.NoError(t, conn1.Write(ctx, websocket.MessageText, bytes))

	conn2, read2, closer2 := help.NewSocket(ctx, t)
	defer closer2()
	require.NoError(t, conn2.Write(ctx, websocket.MessageText, bytes))

	conn3, _, closer3 := help.NewSocket(ctx, t)
	defer closer3()
	help.Publish(ctx, t, eventSent, conn3)

	e, found := help.ReadEvent(ctx, t, read1, messages.SubscriptionID(subID), time.Second)
	require.True(t, found)
	require.Equal(t, eventSent.ID, e.ID)

	e, found = help.ReadEvent(ctx, t, read2, messages.SubscriptionID(subID), time.Second)
	require.True(t, found)
	require.Equal(t, eventSent.ID, e.ID)
}

func TestNIP01DuplicateSubscriptionIDsBetweenSessionsClosing(t *testing.T) {
	ctx := context.Background()
	subID := messages.SubscriptionID(uuid.NewString())
	eventSent := help.Event(t)
	bytes, err := json.Marshal([]interface{}{"REQ", subID, messages.Filter{IDs: []event.ID{eventSent.ID}}})
	require.NoError(t, err)

	conn1, read1, closer1 := help.NewSocket(ctx, t)
	defer closer1()
	require.NoError(t, conn1.Write(ctx, websocket.MessageText, bytes))

	conn2, _, closer2 := help.NewSocket(ctx, t)
	defer closer2()
	require.NoError(t, conn2.Write(ctx, websocket.MessageText, bytes))

	help.CloseSubscription(ctx, t, subID, conn2)

	conn3, _, closer3 := help.NewSocket(ctx, t)
	defer closer3()
	help.Publish(ctx, t, eventSent, conn3)

	e, found := help.ReadEvent(ctx, t, read1, subID, time.Second)
	require.True(t, found)
	require.Equal(t, eventSent.ID, e.ID)
}

func TestNIP01SendEOSE(t *testing.T) {
	ctx, ctxCloser := context.WithTimeout(context.Background(), time.Second)
	defer ctxCloser()
	conn, read, connCloser := help.NewSocket(ctx, t)
	defer connCloser()
	subWithouthMatches, subCloser := help.RequestSub(ctx, t, conn,
		messages.Filter{IDs: []event.ID{event.ID(uuid.NewString())}},
	)
	defer subCloser()
	_, err := help.StoredEvents(t, read, subWithouthMatches, time.Second)
	require.NoError(t, err)
}
