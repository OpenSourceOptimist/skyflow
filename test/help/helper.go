package help

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/google/uuid"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

type Closer func()

func WithURI(uri string) SocketOpts {
	return SocketOpts{URI: uri}
}

type SocketOpts struct {
	URI string
}

var defaultURI string = "ws://localhost:80"

func SetDefaultURI(uri string) {
	defaultURI = uri
}

func NewSocket(ctx context.Context, t require.TestingT, opts ...SocketOpts) (*websocket.Conn, Closer) {
	uri := defaultURI
	for _, opt := range opts {
		if opt.URI != "" {
			uri = opt.URI
		}
	}
	conn, resp, err := websocket.Dial(ctx, uri, &websocket.DialOptions{CompressionMode: websocket.CompressionDisabled})
	require.NoError(t, err, "websocket dial error")
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "handshake status")
	return conn, func() { conn.Close(websocket.StatusGoingAway, "bye") }
}

func toEvent(ne nostr.Event) event.Event {
	return event.Event{
		ID:        event.ID(ne.ID),
		PubKey:    event.PubKey(ne.PubKey),
		CreatedAt: event.Timestamp(ne.CreatedAt.Unix()),
		Kind:      event.Kind(ne.Kind),
		Tags:      slice.Map(ne.Tags, func(t nostr.Tag) event.Tag { return event.Tag(t) }),
		Content:   ne.Content,
		Sig:       event.Signature(ne.Sig),
	}
}

func Publish(ctx context.Context, t require.TestingT, e event.Event, conn *websocket.Conn) {
	reqBytes, err := json.Marshal([]interface{}{"EVENT", e})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))
}

func RequestSub(ctx context.Context, t *testing.T, conn *websocket.Conn, filters ...messages.Filter) (messages.SubscriptionID, Closer) {
	if len(filters) == 0 {
		filters = []messages.Filter{{}}
	}
	subID := uuid.NewString()
	requestMsg := []interface{}{"REQ", subID}
	for _, filter := range filters {
		requestMsg = append(requestMsg, filter)
	}
	bytes, err := json.Marshal(requestMsg)
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, bytes))
	typedSubID := messages.SubscriptionID(subID)
	return typedSubID, func() { CloseSubscription(ctx, t, typedSubID, conn) }
}

func GetEvent(ctx context.Context, t *testing.T, conn *websocket.Conn, id event.ID, timeout time.Duration) (event.Event, bool) {
	subID, _ := RequestSub(ctx, t, conn, messages.Filter{IDs: []event.ID{id}})
	select {
	case e := <-ListenForEventsOnSub(ctx, t, conn, subID):
		return e, true
	case <-time.After(timeout):
		return event.Event{}, false
	}
}

func ListenForEventsOnSub(
	ctx context.Context, t *testing.T, conn *websocket.Conn, sub messages.SubscriptionID,
) <-chan event.Event {
	events := make(chan event.Event)
	go func() {
		for {
			id, e, err := ReadEvent(ctx, t, conn)
			if err != nil {
				close(events)
				return
			}
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
func ReadEvent(ctx context.Context, t *testing.T, conn *websocket.Conn) (messages.SubscriptionID, event.Event, error) {
	msgType, responseBytes, err := conn.Read(ctx)
	if err != nil {
		return messages.SubscriptionID(""), event.Event{}, err
	}
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
	return subscriptionID, resultEvent, nil
}

func CloseSubscription(ctx context.Context, t *testing.T, subID messages.SubscriptionID, conn *websocket.Conn) {
	bytes, err := json.Marshal([]interface{}{"CLOSE", subID})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, bytes))
}
