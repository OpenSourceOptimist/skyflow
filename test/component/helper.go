package component

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/google/uuid"
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

func readEvent(ctx context.Context, t *testing.T, conn *websocket.Conn) event.Event {
	msgType, responseBytes, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, msgType)

	var eventDataMsg []json.RawMessage
	require.NoError(t, json.Unmarshal(responseBytes, &eventDataMsg))
	require.Len(t, eventDataMsg, 2)
	var recivedMsgType string
	require.NoError(t, json.Unmarshal(eventDataMsg[0], &recivedMsgType))
	require.Equal(t, "EVENT", recivedMsgType)
	var resultEvent event.Event
	require.NoError(t, json.Unmarshal(eventDataMsg[1], &resultEvent))
	require.NoError(t, event.VerifyEvent(resultEvent), "verifying validity of recived event")
	return resultEvent
}

func cancelSub(ctx context.Context, t *testing.T, subID messages.SubscriptionID, conn *websocket.Conn) {
	bytes, err := json.Marshal([]interface{}{"CLOSE", subID})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, bytes))
}
