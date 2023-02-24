package component

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

func TestFail(t *testing.T) {
	ctx := context.Background()

	conn, resp, err := websocket.Dial(ctx, "ws://localhost:80", nil)
	require.NoError(t, err, "websocket dial error")
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "handshake status")

	e := event.Event{
		Content: "hello world",
	}
	bytes, err := json.Marshal(e)
	require.NoError(t, err)

	require.NoError(t, conn.Write(ctx, websocket.MessageText, bytes))

	msgType, responseBytes, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, msgType)

	var respEvent event.Event
	require.NoError(t, json.Unmarshal(responseBytes, &respEvent))
	require.Equal(t, "hello world", respEvent.Content)
}
