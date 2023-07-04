package help

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
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

var defaultURI string = "ws://127.0.0.1:80"

func SetDefaultURI(uri string) {
	defaultURI = uri
}

type MsgRead []byte

type MsgWriter struct {
	conn *websocket.Conn
}

func (w *MsgWriter) Write(ctx context.Context, typ websocket.MessageType, p []byte) error {
	//fmt.Println("write:" + string(p))
	return w.conn.Write(ctx, typ, p)
}

func NewSocket(ctx context.Context, t require.TestingT, opts ...SocketOpts) (MsgWriter, <-chan MsgRead, Closer) {
	uri := defaultURI
	for _, opt := range opts {
		if opt.URI != "" {
			uri = opt.URI
		}
	}
	conn, resp, err := websocket.Dial(ctx, uri, &websocket.DialOptions{CompressionMode: websocket.CompressionDisabled})
	require.NoError(t, err, "websocket dial error")
	if resp == nil {
		t.FailNow()
		return MsgWriter{}, nil, func() {}
	}
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "handshake status")
	read := make(chan MsgRead)
	go func() {
		var wg sync.WaitGroup
		defer func() {
			wg.Wait()
			close(read)
		}()
		for {
			msgType, msg, err := conn.Read(ctx)
			if err != nil {
				return
			}
			//fmt.Println("read:" + string(msg))
			require.Equal(t, websocket.MessageText, msgType)
			wg.Add(1)
			go func(b []byte) {
				read <- MsgRead(b)
				wg.Done()
			}(msg)
		}
	}()
	return MsgWriter{conn: conn}, read, func() { conn.Close(websocket.StatusGoingAway, "bye") }
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

func Publish(ctx context.Context, t require.TestingT, e event.Event, conn MsgWriter) {
	reqBytes, err := json.Marshal([]interface{}{"EVENT", e})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, reqBytes))
}

func RequestSub(ctx context.Context, t *testing.T, conn MsgWriter, filters ...messages.Filter) (messages.SubscriptionID, Closer) {
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

func GetEvent(ctx context.Context, t *testing.T, id event.ID) (event.Event, bool) {
	conn, read, closer := NewSocket(ctx, t)
	defer closer()
	subID, _ := RequestSub(ctx, t, conn, messages.Filter{IDs: []event.ID{id}})
	return ReadEvent(ctx, t, read, subID)
}

func StoredEvents(
	t *testing.T,
	read <-chan MsgRead,
	sub messages.SubscriptionID,
	timeout time.Duration) ([]event.Event, error) {
	events := make([]event.Event, 0)
	giveUp := time.After(timeout)
	for {
		var responseBytes MsgRead
		select {
		case responseBytes = <-read:
		case <-giveUp:
			return nil, fmt.Errorf("timeout")
		}
		var eventDataMsg []json.RawMessage
		require.NoError(t, json.Unmarshal(responseBytes, &eventDataMsg))
		var recivedMsgType string
		require.NoError(t, json.Unmarshal(eventDataMsg[0], &recivedMsgType))
		if recivedMsgType == "EOSE" {
			var subscriptionID messages.SubscriptionID
			require.NoError(t, json.Unmarshal(eventDataMsg[1], &subscriptionID))
			if subscriptionID == sub {
				return events, nil
			}
		}
		if recivedMsgType != "EVENT" {
			fmt.Println(string(responseBytes))
			continue
		}
		require.Len(t, eventDataMsg, 3)
		var subscriptionID messages.SubscriptionID
		require.NoError(t, json.Unmarshal(eventDataMsg[1], &subscriptionID))
		var resultEvent event.Event
		require.NoError(t, json.Unmarshal(eventDataMsg[2], &resultEvent))
		require.NoError(t, event.VerifyEvent(resultEvent), "verifying validity of recived event")
		events = append(events, resultEvent)
	}
}

func ListenForEventsOnSub(
	ctx context.Context,
	t *testing.T,
	read <-chan MsgRead,
	sub messages.SubscriptionID) chan event.Event {
	result := make(chan event.Event)
	go func() {
		for {
			e, found := ReadEvent(ctx, t, read, sub)
			if !found {
				close(result)
				return
			}
			result <- e
		}
	}()
	return result
}

func ReadEvent(ctx context.Context, t *testing.T, read <-chan MsgRead, sub messages.SubscriptionID) (event.Event, bool) {
	for responseBytes := range read {
		var eventDataMsg []json.RawMessage
		require.NoError(t, json.Unmarshal(responseBytes, &eventDataMsg))
		var recivedMsgType string
		require.NoError(t, json.Unmarshal(eventDataMsg[0], &recivedMsgType))
		if recivedMsgType != "EVENT" {
			continue
		}
		require.Equal(t, "EVENT", recivedMsgType)
		require.Len(t, eventDataMsg, 3)
		var subscriptionID messages.SubscriptionID
		require.NoError(t, json.Unmarshal(eventDataMsg[1], &subscriptionID))
		if subscriptionID != sub {
			continue
		}
		var resultEvent event.Event
		require.NoError(t, json.Unmarshal(eventDataMsg[2], &resultEvent))
		require.NoError(t, event.VerifyEvent(resultEvent), "verifying validity of recived event")
		return resultEvent, true
	}
	return event.Event{}, false
}

func CloseSubscription(ctx context.Context, t *testing.T, subID messages.SubscriptionID, conn MsgWriter) {
	bytes, err := json.Marshal([]interface{}{"CLOSE", subID})
	require.NoError(t, err)
	require.NoError(t, conn.Write(ctx, websocket.MessageText, bytes))
}
