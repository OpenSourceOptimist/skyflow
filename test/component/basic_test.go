package component

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

func TestBasicNIP01Flow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, "ws://localhost:80", nil)
	require.NoError(t, err, "websocket dial error")
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "handshake status")
	defer conn.Close(websocket.StatusGoingAway, "bye")

	testEvent := event.Event{
		ID:        event.EventID("d7dd5eb3ab747e16f8d0212d53032ea2a7cadef53837e5a6c66d42849fcb9027"),
		Kind:      event.EventKind(1),
		PubKey:    event.PubKey("22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c"),
		CreatedAt: event.Timestamp(1670869179),
		Content:   "NOSTR \"WINE-ACCOUNT\" WITH HARVEST DATE STAMPED\n\n\n\"The older the wine, the greater its reputation\"\n\n\n22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c\n\n\nNWA 2022-12-12\nAA",
		Tags:      []event.EventTag{{"client", "astral"}},
		Sig:       event.EventSignature("f110e4fdf67835fb07abc72469933c40bdc7334615610cade9554bf00945a1cebf84f8d079ec325d26fefd76fe51cb589bdbe208ac9cdbd63351ddad24a57559"),
	}

	reqBytes, err := json.Marshal([]interface{}{"EVENT", testEvent})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))

	subscriptionID := uuid.NewString()
	reqBytes, err = json.Marshal([]interface{}{"REQ", subscriptionID, messages.RequestFilter{IDs: []event.EventID{testEvent.ID}}})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))

	msgType, responseBytes, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, websocket.MessageText, msgType)

	//var respEvent event.Event
	var eventDataMsg [][]byte
	require.NoError(t, json.Unmarshal(responseBytes, &eventDataMsg))
	require.Len(t, eventDataMsg, 3)
	require.Equal(t, "EVENT", string(eventDataMsg[0]))
}
