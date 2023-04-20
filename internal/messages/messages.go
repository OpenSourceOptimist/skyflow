package messages

import (
	"encoding/json"
	"fmt"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SubscriptionID string
type SessionID string
type SubscriptionUUID string
type Subscription struct {
	ID        SubscriptionID `bson:"id"`
	Filters   []Filter       `bson:"filter"`
	SessionID SessionID      `bson:"session"`
}

func (e Subscription) UUID() SubscriptionUUID {
	return GenerateSubscriptionUUID(e.ID, e.SessionID)
}

func GenerateSubscriptionUUID(id SubscriptionID, session SessionID) SubscriptionUUID {
	return SubscriptionUUID(string(id) + string(session))
}

type Filter struct {
	IDs     []event.ID      `json:"ids,omitempty" bson:"ids"`
	Authors []event.PubKey  `json:"authors,omitempty" bson:"authors"`
	Kinds   []event.Kind    `json:"kinds,omitempty" bson:"kinds"`
	E       []event.ID      `json:"#e,omitempty" bson:"#e"`
	P       []event.PubKey  `json:"#p,omitempty" bson:"#p"`
	Since   event.Timestamp `json:"since,omitempty" bson:"since"`
	Until   event.Timestamp `json:"until,omitempty" bson:"until"`
	Limit   int64           `json:"limit,omitempty" bson:"limit"`
}

func (e Subscription) UniqueMatch() primitive.M {
	return primitive.M{"$and": primitive.A{
		primitive.M{"id": e.ID},
		primitive.M{"session": e.SessionID},
	}}
}

type M primitive.M
type A primitive.A

// MongoDB filter for subscriptions that would match event.
func SubscriptionFilter(e event.Event) primitive.M {
	return primitive.M{
		"$and": A{
			M{"$or": A{
				M{"filter.ids": e.ID},
				M{"filter.ids": primitive.Null{}},
			}},
			M{"$or": A{
				M{"filter.authors": e.PubKey},
				M{"filter.authors": primitive.Null{}},
			}},
			M{"$or": A{
				M{"filter.kinds": e.Kind},
				M{"filter.kinds": primitive.Null{}},
			}},
			M{"$or": append(
				slice.Map(slice.FindAll(e.Tags, event.E), func(id event.ID) M { return M{"filter.#e": id} }),
				M{"filter.#e": primitive.Null{}},
			)},
			M{"$or": append(
				slice.Map(slice.FindAll(e.Tags, event.P), func(p event.PubKey) M { return M{"filter.#p": p} }),
				M{"filter.#e": primitive.Null{}},
			)},
		},
	}
}

// MongoDB filter for StructuredEvents matching filter.
func EventFilter(filter Filter) primitive.M {
	var filters []M
	if len(filter.IDs) > 0 {
		idFilters := make([]M, 0, len(filter.IDs))
		for _, id := range filter.IDs {
			idFilters = append(idFilters, M{"event.id": M{"$regex": primitive.Regex{
				Pattern: fmt.Sprintf("^%s.*", id),
			}}})
		}
		filters = append(filters, M{"$or": idFilters})
	}
	if len(filter.Authors) > 0 {
		authorsFilters := make([]M, 0, len(filter.Authors))
		for _, pubKey := range filter.Authors {
			authorsFilters = append(authorsFilters, M{"event.pubkey": M{"$regex": primitive.Regex{
				Pattern: fmt.Sprintf("^%s.*", pubKey),
			}}})
		}
		filters = append(filters, M{"$or": authorsFilters})
	}
	if len(filter.Kinds) > 0 {
		filters = append(filters, M{"event.kind": primitive.M{"$in": filter.Kinds}})
	}
	if len(filter.E) > 0 {
		filters = append(filters, M{"$or": slice.Map(filter.E, func(id event.ID) M { return M{"#e": id} })})
	}
	if len(filter.P) > 0 {
		filters = append(filters, M{"$or": slice.Map(filter.P, func(npub event.PubKey) M { return M{"#p": npub} })})
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

// SessionID is needed to make the requests globaly unique.
func (msg WebsocketMessage) AsREQ(session SessionID) (Subscription, bool) {
	sub, ok := msg.Value.(Subscription)
	if !ok {
		return Subscription{}, false
	}
	sub.SessionID = session
	return sub, true
}

func (msg WebsocketMessage) AsCLOSE(session SessionID) (SubscriptionUUID, bool) {
	sub, ok := msg.Value.(SubscriptionID)
	if !ok {
		return "", false
	}
	return GenerateSubscriptionUUID(sub, session), true
}

type DebugLogger interface {
	Debug(msg string, keyVals ...interface{})
}

func errorMsgf(format string, a ...any) WebsocketMessage {
	return WebsocketMessage{Err: fmt.Errorf(format, a...)}
}

func ParseWebsocketMsg(data []byte) WebsocketMessage {
	var message []json.RawMessage
	err := json.Unmarshal(data, &message)
	if err != nil {
		return errorMsgf("unmarshal message: %w: %s", err, data)
	}
	if len(message) == 0 {
		return errorMsgf("empty message")
	}
	var msgType string
	err = json.Unmarshal(message[0], &msgType)
	if err != nil {
		return errorMsgf("unmarshalling message type: %w", err)
	}
	switch msgType {
	case "EVENT":
		if len(message) != 2 {
			return errorMsgf("wrong event length: %s", string(data))
		}
		var e event.Event
		err := json.Unmarshal(message[1], &e)
		if err != nil {
			return errorMsgf("unmarshal event message: %s", string(data))
		}
		err = event.VerifyEvent(e)
		if err != nil {
			return errorMsgf("event verification: %w", err)
		}
		return WebsocketMessage{MsgType: EVENT, Value: e}
	case "REQ":
		if len(message) < 3 {
			return errorMsgf("wrong event request lenght: %s", string(data))
		}
		var subID SubscriptionID
		err := json.Unmarshal(message[1], &subID)
		if err != nil {
			return errorMsgf("unmatshal sub id: %w", err)
		}
		subscription := Subscription{ID: subID}
		filters := make([]Filter, 0, len(message[2:]))
		for _, element := range message[2:] {
			var filter Filter
			err := json.Unmarshal(element, &filter)
			if err != nil {
				return errorMsgf("unmarshal request message: %s", string(data))
			}
			filters = append(filters, filter)
		}
		subscription.Filters = filters
		return WebsocketMessage{MsgType: REQ, Value: subscription}
	case "CLOSE":
		if len(message) != 2 {
			return errorMsgf("wrong close message lenght: %s", string(data))
		}
		var subscriptionID SubscriptionID
		err := json.Unmarshal(message[1], &subscriptionID)
		if err != nil {
			return errorMsgf("unmatshal sub id: %w", err)
		}
		return WebsocketMessage{MsgType: CLOSE, Value: subscriptionID}
	default:
		return errorMsgf("unknown msg type: %s", msgType)
	}
}
