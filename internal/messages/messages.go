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
type M primitive.M
type A primitive.A

// MongoDB filter for subscriptions that would match event.
func SubscriptionFilter(e event.Event) primitive.M {
	return primitive.M{
		"$and": A{
			M{"$or": A{
				M{"ids": e.ID},
				M{"ids": M{"$size": 0}},
			}},
			M{"$or": A{
				M{"authors": e.PubKey},
				M{"authors": M{"$size": 0}},
			}},
			M{"$or": A{
				M{"kinds": e.Kind},
				M{"kinds": M{"$size": 0}},
			}},
			M{"$or": A{
				M{"$or": slice.Map(
					slice.FindAll(e.Tags, event.E),
					func(id event.ID) M { return M{"tag_e": id} },
				)},
				M{"tag_e": M{"$size": 0}},
			}},
			M{"$or": A{
				M{"$or": slice.Map(
					slice.FindAll(e.Tags, event.P),
					func(key event.PubKey) M { return M{"tag_p": key} },
				)},
				M{"tag_p": M{"$size": 0}},
			}},
			// TODO: missing since, untill
		},
	}
}

// MongoDB filter for events matching filter.
func EventFilter(filter Subscription) primitive.M {
	var filters []primitive.M
	if len(filter.IDs) > 0 {
		filters = append(filters, primitive.M{"id": primitive.M{"$in": filter.IDs}})
	}
	if len(filter.Authors) > 0 {
		filters = append(filters, primitive.M{"pubkey": primitive.M{"$in": filter.Authors}})
	}
	if len(filter.Kinds) > 0 {
		filters = append(filters, primitive.M{"kind": primitive.M{"$in": filter.Kinds}})
	}
	var createdAtConstraints []primitive.M
	if filter.Since != 0 {
		createdAtConstraints = append(createdAtConstraints, primitive.M{"created_at": primitive.M{"$gt": filter.Since}})
	}
	if filter.Until != 0 {
		createdAtConstraints = append(createdAtConstraints, primitive.M{"created_at": primitive.M{"$lt": filter.Until}})
	}
	query := primitive.M{}
	if len(filters) > 0 && len(createdAtConstraints) == 0 {
		query = primitive.M{"$or": filters}
	}
	if len(filters) == 0 && len(createdAtConstraints) > 0 {
		query = primitive.M{"$and": createdAtConstraints}
	}
	if len(filters) > 0 && len(createdAtConstraints) > 0 {
		query = primitive.M{"$and": primitive.A{
			primitive.M{"$and": createdAtConstraints},
			primitive.M{"$or": filters},
		}}
	}
	return query
}

type SubscriptionID string

type CloseMsg struct {
	Subscription SubscriptionID
}

type MessageReader interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
}

func ListenForMessages(ctx context.Context, r MessageReader) (<-chan event.Event, <-chan Subscription, <-chan CloseMsg, <-chan error) {
	events := make(chan event.Event)
	subscriptions := make(chan Subscription)
	closes := make(chan CloseMsg)
	errs := make(chan error)
	go func() {
		for {
			err := ctx.Err()
			if err != nil {
				errs <- fmt.Errorf("context cancelled: %w", err)
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
				errs <- fmt.Errorf("websocket read error: %w", err)
				// TODO: this should probably be exponential in some smart way
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if socketMsgType != websocket.MessageText {
				errs <- fmt.Errorf("unexpected message type: %d", socketMsgType)
				continue
			}
			var message []json.RawMessage
			err = json.Unmarshal(data, &message)
			if err != nil {
				errs <- fmt.Errorf("unmarshal message: %w: %s", err, data)
				continue
			}
			if len(message) == 0 {
				errs <- fmt.Errorf("empty message")
				continue
			}
			var msgType string
			err = json.Unmarshal(message[0], &msgType)
			if err != nil {
				errs <- fmt.Errorf("unmarshalling message type: %w", err)
				continue
			}
			switch msgType {
			case "EVENT":
				if len(message) != 2 {
					errs <- fmt.Errorf("wrong event length: %s", string(data))
					continue
				}
				var e event.Event
				err := json.Unmarshal(message[1], &e)
				if err != nil {
					errs <- fmt.Errorf("unmarshal event message: %s", string(data))
					continue
				}
				err = event.VerifyEvent(e)
				if err != nil {
					errs <- fmt.Errorf("event verification: %w", err)
					continue
				}
				go func(eventToSend event.Event) {
					events <- eventToSend
				}(e)
			case "REQ":
				if len(message) != 3 {
					errs <- fmt.Errorf("wrong event request lenght: %s", string(data))
					continue
				}
				var subID SubscriptionID
				err = json.Unmarshal(message[1], &subID)
				if err != nil {
					errs <- fmt.Errorf("unmatshal sub id: %w", err)
					continue
				}
				var subscription Subscription
				err := json.Unmarshal(message[2], &subscription)
				if err != nil {
					errs <- fmt.Errorf("unmarshal request message: %s", string(data))
					continue
				}
				subscription.ID = subID
				go func(sub Subscription) {
					subscriptions <- sub
				}(subscription)
			case "CLOSE":
				if len(message) != 2 {
					errs <- fmt.Errorf("wrong close message lenght: %s", string(data))
					continue
				}
				go func(subID SubscriptionID) {
					closes <- CloseMsg{Subscription: subID}
				}(SubscriptionID(message[1]))
			}
		}
	}()
	return events, subscriptions, closes, errs
}
