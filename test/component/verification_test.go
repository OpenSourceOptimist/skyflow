package component

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

func TestVerificationEventID(t *testing.T) {
	defer clearMongo()
	ctx := context.Background()
	conn, _, _ := websocket.Dial(ctx, "ws://localhost:80", nil)
	defer conn.Close(websocket.StatusGoingAway, "bye")
	testEvent := event.Event{
		ID:        event.EventID("d8dd5eb23b747e16f8d0212d53032ea2a7cadef53837e5a6c66142843fcb9027"), // modified in a couple of places
		Kind:      event.EventKind(1),
		PubKey:    event.PubKey("22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c"),
		CreatedAt: event.Timestamp(1670869179),
		Content:   "NOSTR \"WINE-ACCOUNT\" WITH HARVEST DATE STAMPED\n\n\n\"The older the wine, the greater its reputation\"\n\n\n22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c\n\n\nNWA 2022-12-12\nAA",
		Tags:      []event.EventTag{{"client", "astral"}},
		Sig:       event.EventSignature("f110e4fdf67835fb07abc72469933c40bdc7334615610cade9554bf00945a1cebf84f8d079ec325d26fefd76fe51cb589bdbe208ac9cdbd63351ddad24a57559"),
	}

	// Writing a bad event should work
	eventMsg, _ := json.Marshal([]any{"EVENT", testEvent})

	require.NoError(t, conn.Write(ctx, websocket.MessageText, eventMsg))
	// But should not show up in a request
	reqBytes, _ := json.Marshal([]interface{}{"REQ", uuid.NewString(), messages.RequestFilter{IDs: []event.EventID{testEvent.ID}}})
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))
	ctx, cancelFunc := context.WithTimeout(ctx, time.Second)
	defer cancelFunc()
	_, readBytes, err := conn.Read(ctx)
	require.Equal(t, "", string(readBytes))
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestVerificationSignature(t *testing.T) {
	defer clearMongo()
	ctx := context.Background()
	conn, _, _ := websocket.Dial(ctx, "ws://localhost:80", nil)
	defer conn.Close(websocket.StatusGoingAway, "bye")
	testEvent := event.Event{
		ID:        event.EventID("d7dd5eb3ab747e16f8d0212d53032ea2a7cadef53837e5a6c66d42849fcb9027"),
		Kind:      event.EventKind(1),
		PubKey:    event.PubKey("22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c"),
		CreatedAt: event.Timestamp(1670869179),
		Content:   "NOSTR \"WINE-ACCOUNT\" WITH HARVEST DATE STAMPED\n\n\n\"The older the wine, the greater its reputation\"\n\n\n22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c\n\n\nNWA 2022-12-12\nAA",
		Tags:      []event.EventTag{{"client", "astral"}},
		Sig:       event.EventSignature("f111e4fdf67835fb07abc72469933c40bdc7334615610cade9554bf00945a1cebf84f8d079ec325d26fefd76fe51cb589bdbe208ac9cdbd63351ddad24a57559"), // modified
	}

	// Writing a bad event should work
	eventMsg, _ := json.Marshal([]any{"EVENT", testEvent})
	require.NoError(t, conn.Write(ctx, websocket.MessageText, eventMsg))
	// But should not show up in a request
	reqBytes, _ := json.Marshal([]interface{}{"REQ", uuid.NewString(), messages.RequestFilter{IDs: []event.EventID{testEvent.ID}}})
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))
	ctx, cancelFunc := context.WithTimeout(ctx, time.Second)
	defer cancelFunc()
	_, readBytes, err := conn.Read(ctx)
	require.Equal(t, "", string(readBytes))
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
