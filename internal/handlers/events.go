package handlers

import (
	"context"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func HandleEventMessage(
	ctx context.Context,
	msg ConvertableToEvent,
	eventsDB DbEventInserter,
	subscriptionsDB DBSubscriptionFinder,
	ongoingSubscriptions SubcriptionHandleLoader,
) {
	e, ok := msg.AsEvent()
	if !ok {
		return
	}
	err := eventsDB.InsertOne(ctx, event.Structure(e))
	if err != nil {
		logrus.Error("mongo error on inserting new event: " + err.Error())
	}
	for subscription := range subscriptionsDB.Find(ctx, messages.SubscriptionFilter(e)) {
		handle, ok := ongoingSubscriptions.Load(subscription.UUID())
		if !ok {
			// Possibly we should remove the subsction
			// from the DB here. Not sure.
			continue
		}
		slice.AsyncWrite(handle.Ctx, handle.NewEvents, e)
	}
}
