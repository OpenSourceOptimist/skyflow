package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"nhooyr.io/websocket"
)

type Subscription struct {
	ID      SubscriptionID  `json:"id" bson:"id"`
	IDs     []event.ID      `json:"ids" bson:"ids"`
	Authors []event.PubKey  `json:"authors" bson:"authors"`
	Kinds   []event.Kind    `json:"kinds" bson:"kinds"`
	E       []event.ID      `json:"#e" bson:"tag_e"`
	P       []event.PubKey  `json:"#p" bson:"tag_p"`
	Since   event.Timestamp `json:"since" bson:"since"`
	Until   event.Timestamp `json:"until" bson:"until"`
	Limit   int64           `json:"limit" bson:"limit"`
}

func (e Subscription) UniqueID() string {
	return string(e.ID)
}
func (e Subscription) UniqueIDFieldName(format string) (string, error) {
	if format == "json" || format == "bson" {
		return "id", nil
	} else if format == "struct" {
		return "ID", nil
	}
	return "", fmt.Errorf("only support format json, bson, and struct, not " + format)
}

type M primitive.M
type A primitive.A

// MongoDB filter for subscriptions that would match event.
func SubscriptionFilter(e event.Event) primitive.M {
	return primitive.M{
		"$and": A{
			M{"$or": A{
				M{"ids": e.ID},
				M{"ids": primitive.Null{}},
			}},
			M{"$or": A{
				M{"authors": e.PubKey},
				M{"authors": primitive.Null{}},
			}},
			M{"$or": A{
				M{"kinds": e.Kind},
				M{"kinds": primitive.Null{}},
			}},
			M{"$or": append(
				slice.Map(slice.FindAll(e.Tags, event.E), func(id event.ID) M { return M{"tag_e": id} }),
				M{"tag_e": primitive.Null{}},
			)},
			M{"$or": append(
				slice.Map(slice.FindAll(e.Tags, event.P), func(p event.PubKey) M { return M{"tag_p": p} }),
				M{"tag_e": primitive.Null{}},
			)},
		},
	}
}

// MongoDB filter for StructuredEvents matching filter.
func EventFilter(filter Subscription) primitive.M {
	var filters []M
	if len(filter.IDs) > 0 {
		filters = append(filters, M{"event.id": primitive.M{"$in": filter.IDs}})
	}
	if len(filter.Authors) > 0 {
		filters = append(filters, M{"event.pubkey": primitive.M{"$in": filter.Authors}})
	}
	if len(filter.Kinds) > 0 {
		filters = append(filters, M{"event.kind": primitive.M{"$in": filter.Kinds}})
	}
	if filter.Since != 0 {
		filters = append(filters, M{"event.created_at": primitive.M{"$gt": filter.Since}})
	}
	if filter.Until != 0 {
		filters = append(filters, M{"event.created_at": primitive.M{"$lt": filter.Until}})
	}
	query := primitive.M{}
	if len(filters) > 0 {
		query = primitive.M{"$and": filters}
	}
	return query
}

type SubscriptionID string

type MessageReader interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
}
type MessageType string

const (
	EVENT = "EVENT"
	REQ   = "REQ"
	CLOSE = "CLOSE"
)

type WebsocketMessage struct {
	MsgType MessageType
	Err     error
	Value   interface{}
}

func (msg WebsocketMessage) AsEvent() (event.Event, bool) {
	e, ok := msg.Value.(event.Event)
	return e, ok
}

func (msg WebsocketMessage) AsREQ() (Subscription, bool) {
	sub, ok := msg.Value.(Subscription)
	return sub, ok
}

func (msg WebsocketMessage) AsCLOSE() (SubscriptionID, bool) {
	sub, ok := msg.Value.(SubscriptionID)
	return sub, ok
}

func ListenForMessages(ctx context.Context, r MessageReader) <-chan WebsocketMessage {
	result := make(chan WebsocketMessage)
	go func() {
		for {
			err := ctx.Err()
			if err != nil {
				result <- WebsocketMessage{Err: fmt.Errorf("context cancelled: %w", err)}
				return
			}
			socketMsgType, data, err := r.Read(ctx)
			if err != nil {
				logrus.Debug("read error: " + err.Error())
				if strings.Contains(err.Error(), "WebSocket closed") {
					return
				}
				if strings.Contains(err.Error(), "connection reset by peer") {
					return
				}
				if strings.Contains(err.Error(), "StatusGoingAway") {
					return
				}
				if strings.Contains(err.Error(), "EOF") {
					return
				}
				result <- WebsocketMessage{Err: fmt.Errorf("websocket read error: %w", err)}
				// TODO: this should probably be exponential in some smart way
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if socketMsgType != websocket.MessageText {
				result <- WebsocketMessage{Err: fmt.Errorf("unexpected message type: %d", socketMsgType)}
				continue
			}
			var message []json.RawMessage
			err = json.Unmarshal(data, &message)
			if err != nil {
				result <- WebsocketMessage{Err: fmt.Errorf("unmarshal message: %w: %s", err, data)}
				continue
			}
			if len(message) == 0 {
				result <- WebsocketMessage{Err: fmt.Errorf("empty message")}
				continue
			}
			var msgType string
			err = json.Unmarshal(message[0], &msgType)
			if err != nil {
				result <- WebsocketMessage{Err: fmt.Errorf("unmarshalling message type: %w", err)}
				continue
			}
			switch msgType {
			case "EVENT":
				if len(message) != 2 {
					result <- WebsocketMessage{Err: fmt.Errorf("wrong event length: %s", string(data))}
					continue
				}
				var e event.Event
				err := json.Unmarshal(message[1], &e)
				if err != nil {
					result <- WebsocketMessage{Err: fmt.Errorf("unmarshal event message: %s", string(data))}
					continue
				}
				err = event.VerifyEvent(e)
				if err != nil {
					result <- WebsocketMessage{Err: fmt.Errorf("event verification: %w", err)}
					continue
				}
				go func(eventToSend event.Event) {
					result <- WebsocketMessage{MsgType: EVENT, Value: eventToSend}
				}(e)
			case "REQ":
				if len(message) != 3 {
					result <- WebsocketMessage{Err: fmt.Errorf("wrong event request lenght: %s", string(data))}
					continue
				}
				var subID SubscriptionID
				err = json.Unmarshal(message[1], &subID)
				if err != nil {
					result <- WebsocketMessage{Err: fmt.Errorf("unmatshal sub id: %w", err)}
					continue
				}
				var subscription Subscription
				err := json.Unmarshal(message[2], &subscription)
				if err != nil {
					result <- WebsocketMessage{Err: fmt.Errorf("unmarshal request message: %s", string(data))}
					continue
				}
				subscription.ID = subID
				go func(sub Subscription) {
					result <- WebsocketMessage{MsgType: REQ, Value: sub}
				}(subscription)
			case "CLOSE":
				if len(message) != 2 {
					result <- WebsocketMessage{Err: fmt.Errorf("wrong close message lenght: %s", string(data))}
					continue
				}
				var subscriptionID SubscriptionID
				err = json.Unmarshal(message[1], &subscriptionID)
				if err != nil {
					result <- WebsocketMessage{Err: fmt.Errorf("unmatshal sub id: %w", err)}
					continue
				}
				go func(subID SubscriptionID) {
					result <- WebsocketMessage{MsgType: CLOSE, Value: subID}
				}(subscriptionID)
			}
		}
	}()
	return result
}
