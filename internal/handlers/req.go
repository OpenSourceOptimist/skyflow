package handlers

import (
	"context"
	"encoding/json"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"nhooyr.io/websocket"
)

type ConvertableToREQ interface {
	AsREQ(messages.SessionID) (messages.Subscription, bool)
}
type SubcriptionHandleLoadStorer interface {
	Load(messages.SubscriptionUUID) (messages.SubscriptionHandle, bool)
	Store(messages.SubscriptionUUID, messages.SubscriptionHandle)
}
type DBInserter interface {
	InsertOne(context.Context, messages.Subscription) error
}
type DBFinder interface {
	Find(context.Context, primitive.M, ...store.FindOptions) <-chan event.StructuredEvent
}

func HandleREQ(
	ctx context.Context,
	session messages.SessionID,
	msg ConvertableToREQ,
	globalOngoingSubscriptions SubcriptionHandleLoadStorer,
	subscritptionsDB DBInserter,
	eventDB DBFinder,
	conn *websocket.Conn,
) context.CancelFunc {
	subscriptionCtx, cancelSubscription := context.WithCancel(ctx) //nolint:govet
	subscription, ok := msg.AsREQ(session)
	if !ok {
		return cancelSubscription
	}
	if _, ok := globalOngoingSubscriptions.Load(subscription.UUID()); ok {
		return cancelSubscription
	}
	err := subscritptionsDB.InsertOne(ctx, subscription)
	if err != nil {
		logrus.Error("mongo error on inserting supscription: " + err.Error())
	}
	newEvents := make(chan event.Event)
	globalOngoingSubscriptions.Store(
		subscription.UUID(),
		messages.SubscriptionHandle{
			Ctx:       subscriptionCtx,
			Cancel:    cancelSubscription,
			NewEvents: newEvents,
			Details:   subscription,
		},
	)
	eventsPerFilter := slice.Map(subscription.Filters,
		func(filter messages.Filter) <-chan event.StructuredEvent {
			return eventDB.Find(
				subscriptionCtx,
				messages.EventFilter(filter),
				store.FindOptions{
					Sort:  primitive.D{{Key: "event.created_at", Value: -1}},
					Limit: filter.Limit,
				},
			)
		})
	dbEventsStructured := slice.Merge(subscriptionCtx, eventsPerFilter...)
	dbEvents := slice.MapChan(subscriptionCtx, dbEventsStructured, event.UnStructure)
	dbEventsAsMessages := slice.MapChanSkipErrors(
		ctx, dbEvents, eventToWebsocketMsg(subscription.ID))
	newEventsAsMessages := slice.MapChanSkipErrors(
		ctx, newEvents, eventToWebsocketMsg(subscription.ID))
	subscriptionEvents := slice.ChanConcatenate(
		dbEventsAsMessages,
		slice.AsClosedChan(eoseWebsocketMsg(subscription.ID)),
		newEventsAsMessages)
	go writeToConnection(subscriptionCtx, subscriptionEvents, conn)
	return cancelSubscription
}

func eoseWebsocketMsg(sub messages.SubscriptionID) []byte {
	bytes, err := json.Marshal([]any{"EOSE", sub})
	if err != nil {
		panic("failed to marshal EOSE message")
	}
	return bytes
}
func writeToConnection(ctx context.Context, msgChan <-chan []byte, connection *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case eventMsg := <-msgChan:
			err := connection.Write(ctx, websocket.MessageText, eventMsg)
			if err != nil {
				logrus.Error("websocket write error: " + err.Error())
			}
		}
	}
}

func eventToWebsocketMsg(sub messages.SubscriptionID) func(event.Event) ([]byte, error) {
	return func(e event.Event) ([]byte, error) {
		eventMsg, err := json.Marshal([]any{"EVENT", sub, e})
		if err != nil {
			return nil, err
		}
		return eventMsg, nil
	}
}
