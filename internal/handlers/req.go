package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/otel"
	"nhooyr.io/websocket"
)

type ReqMsg interface {
	AsREQ(messages.SessionID) (messages.Subscription, bool)
}
type LoadStorer interface {
	Load(messages.SubscriptionUUID) (messages.SubscriptionHandle, bool)
	Store(messages.SubscriptionUUID, messages.SubscriptionHandle)
}
type OneInserter interface {
	InsertOne(context.Context, messages.Subscription) error
}
type Finder interface {
	Find(context.Context, primitive.M, ...store.FindOptions) <-chan event.StructuredEvent
}

func Req(
	ctx context.Context,
	session messages.SessionID,
	msg ReqMsg,
	eventDB Finder,
	subscritptionsDB OneInserter,
	globalOngoingSubscriptions LoadStorer,
	conn *websocket.Conn,
) (messages.SubscriptionUUID, context.CancelFunc, bool) {
	ctx, span := otel.Tracer("skyflow").Start(ctx, "HandleREQ")
	defer span.End()
	subscriptionCtx, cancelSubscription := context.WithCancel(ctx) //nolint:govet
	subscription, ok := msg.AsREQ(session)
	if !ok {
		span.RecordError(fmt.Errorf("msg not not REQ as expected"))
		return messages.SubscriptionUUID(""), cancelSubscription, false
	}
	if _, ok := globalOngoingSubscriptions.Load(subscription.UUID()); ok {
		span.RecordError(fmt.Errorf("there is already and ongoing subscription with the same ID for this session"))
		return messages.SubscriptionUUID(""), cancelSubscription, false
	}
	err := subscritptionsDB.InsertOne(ctx, subscription)
	if err != nil {
		span.RecordError(fmt.Errorf("inserting supscription into DB: %w", err))
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
		slice.AsClosedChan(eoseWebsocketMsg(subscriptionCtx, subscription.ID)),
		newEventsAsMessages,
	)
	go writeToConnection(subscriptionCtx, subscriptionEvents, conn)
	return subscription.UUID(), cancelSubscription, true
}

func eoseWebsocketMsg(ctx context.Context, sub messages.SubscriptionID) []byte {
	_, span := otel.Tracer("skyflow").Start(ctx, "handlers_req_eoseWebsocketMsg")
	defer span.End()
	bytes, err := json.Marshal([]any{"EOSE", sub})
	if err != nil {
		span.RecordError(fmt.Errorf("failed to marshal EOSE message: %w", err))
	}
	return bytes
}
func writeToConnection(ctx context.Context, msgChan <-chan []byte, connection *websocket.Conn) {
	ctx, span := otel.Tracer("skyflow").Start(ctx, "handlers_req_writeToConnection")
	defer span.End()
	for {
		select {
		case <-ctx.Done():
			return
		case eventMsg := <-msgChan:
			span.AddEvent("msg on connection")
			err := connection.Write(ctx, websocket.MessageText, eventMsg)
			if err != nil {
				span.RecordError(fmt.Errorf("websocket write error: %w", err))
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
