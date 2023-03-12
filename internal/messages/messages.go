package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"nhooyr.io/websocket"
)

type RequestFilter struct {
	IDs     []event.ID      `json:"ids"`     // prefixes allowed
	Authors []event.PubKey  `json:"authors"` // the pubkey of an event must be one of these, prefixes allowed
	Kinds   []event.Kind    `json:"kinds"`   // a list of a kind numbers
	E       []event.ID      `json:"#e"`      // a list of event ids that are referenced in an "e" tag
	P       []event.PubKey  `json:"#p"`      // a list of pubkeys that are referenced in a "p" tag
	Since   event.Timestamp `json:"since"`   // an integer unix timestamp, events must be newer than this to pass
	Until   event.Timestamp `json:"until"`   // an integer unix timestamp, events must be older than this to pass
	Limit   int64           `json:"limit"`   // maximum number of events to be returned in the initial query
}

func (filter RequestFilter) AsMongoQuery() primitive.M {
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

type RequestMsg struct {
	ID     SubscriptionID
	Filter RequestFilter
}

type CloseMsg struct {
	Subscription SubscriptionID
}

type MessageReader interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
}

func ListenForMessages(ctx context.Context, r MessageReader) (<-chan event.Event, <-chan RequestMsg, <-chan CloseMsg, <-chan error) {
	events := make(chan event.Event)
	requests := make(chan RequestMsg)
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
				var filter RequestFilter
				err := json.Unmarshal(message[2], &filter)
				if err != nil {
					errs <- fmt.Errorf("unmarshal request message: %s", string(data))
					continue
				}
				go func(id SubscriptionID, f RequestFilter) {
					requests <- RequestMsg{ID: id, Filter: f}
				}(subID, filter)
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
	return events, requests, closes, errs
}
