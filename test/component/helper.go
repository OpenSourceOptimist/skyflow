package component

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

type Closer func()

func newSocket(ctx context.Context, t *testing.T) (*websocket.Conn, Closer) {
	conn, resp, err := websocket.Dial(ctx, "ws://localhost:80", nil)
	require.NoError(t, err, "websocket dial error")
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "handshake status")
	return conn, func() { conn.Close(websocket.StatusGoingAway, "bye") }
}

func toEvent(ne nostr.Event) event.Event {
	return event.Event{
		ID:        event.EventID(ne.ID),
		PubKey:    event.PubKey(ne.PubKey),
		CreatedAt: event.Timestamp(ne.CreatedAt.Unix()),
		Kind:      event.EventKind(ne.Kind),
		Tags:      slice.Map(ne.Tags, func(t nostr.Tag) event.EventTag { return event.EventTag(t) }),
		Content:   ne.Content,
		Sig:       event.EventSignature(ne.Sig),
	}

}

func publish(ctx context.Context, t *testing.T, e event.Event, conn *websocket.Conn) {
	reqBytes, err := json.Marshal([]interface{}{"EVENT", e})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))
}

func requestSub(ctx context.Context, t *testing.T, filter messages.RequestFilter, conn *websocket.Conn) messages.SubscriptionID {
	subID := uuid.NewString()
	bytes, err := json.Marshal([]interface{}{"REQ", subID, filter})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, bytes))
	return messages.SubscriptionID(subID)
}

func listenForEventsOnSub(
	ctx context.Context, t *testing.T, conn *websocket.Conn, sub messages.SubscriptionID,
) <-chan event.Event {
	events := make(chan event.Event)
	go func() {
		for {
			id, e := readEvent(ctx, t, conn)
			if id != sub {
				continue
			}
			select {
			case events <- e:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events
}

func readEvent(ctx context.Context, t *testing.T, conn *websocket.Conn) (messages.SubscriptionID, event.Event) {
	msgType, responseBytes, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, msgType)

	var eventDataMsg []json.RawMessage
	require.NoError(t, json.Unmarshal(responseBytes, &eventDataMsg))
	require.Len(t, eventDataMsg, 3)
	var recivedMsgType string
	require.NoError(t, json.Unmarshal(eventDataMsg[0], &recivedMsgType))
	require.Equal(t, "EVENT", recivedMsgType)
	var subscriptionID messages.SubscriptionID
	require.NoError(t, json.Unmarshal(eventDataMsg[1], &subscriptionID))
	var resultEvent event.Event
	require.NoError(t, json.Unmarshal(eventDataMsg[2], &resultEvent))
	require.NoError(t, event.VerifyEvent(resultEvent), "verifying validity of recived event")
	return subscriptionID, resultEvent
}

func cancelSub(ctx context.Context, t *testing.T, subID messages.SubscriptionID, conn *websocket.Conn) {
	bytes, err := json.Marshal([]interface{}{"CLOSE", subID})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, bytes))
}
