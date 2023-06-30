package handlers

import (
	"context"
	"fmt"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.opentelemetry.io/otel"
)

type ConvertableToEvent interface {
	AsEvent() (event.Event, bool)
}

type DbEventInserter interface {
	InsertOne(context.Context, event.StructuredEvent) error
}

type DBSubscriptionFinder interface {
	Find(context.Context, primitive.M, ...store.FindOptions) <-chan messages.Subscription
}

type SubcriptionHandleLoader interface {
	Load(messages.SubscriptionUUID) (messages.SubscriptionHandle, bool)
}

func Event(
	ctx context.Context,
	msg ConvertableToEvent,
	eventsDB DbEventInserter,
	subscriptionsDB DBSubscriptionFinder,
	ongoingSubscriptions SubcriptionHandleLoader,
) {
	ctx, span := otel.Tracer("skyflow").Start(ctx, "HandleEventMessage")
	defer span.End()
	e, ok := msg.AsEvent()
	if !ok {
		span.RecordError(fmt.Errorf("message not convertible to event"))
		return
	}
	err := eventsDB.InsertOne(ctx, event.Structure(e))
	if err != nil {
		span.RecordError(fmt.Errorf("mongo error on inserting new event: %w", err))
	}
	for subscription := range subscriptionsDB.Find(ctx, messages.SubscriptionFilter(e)) {
		handle, ok := ongoingSubscriptions.Load(subscription.UUID())
		if !ok {
			span.RecordError(fmt.Errorf("subscription in DB but no handler found"))
			// Possibly we should remove the subsction
			// from the DB here. Not sure.
			continue
		}
		slice.AsyncWrite(handle.Ctx, handle.NewEvents, e)
	}
}
