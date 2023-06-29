package handlers

import (
	"context"
	"fmt"

	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"go.opentelemetry.io/otel"
)

type CloseMsg interface {
	AsCLOSE(messages.SessionID) (messages.SubscriptionUUID, bool)
}
type Deleter interface {
	DeleteOne(context.Context, messages.Subscription) error
}
type LoadDeleter interface {
	Load(messages.SubscriptionUUID) (messages.SubscriptionHandle, bool)
	Delete(messages.SubscriptionUUID)
}

func Close(
	ctx context.Context,
	session messages.SessionID,
	msg CloseMsg,
	subscriptionDB Deleter,
	globalOngoingSubscriptions LoadDeleter,
) (messages.SubscriptionUUID, bool) {
	ctx, span := otel.Tracer("skyflow").Start(ctx, "handlers_close")
	defer span.End()
	subscriptionToClose, ok := msg.AsCLOSE(session)
	if !ok {
		span.RecordError(fmt.Errorf("msg not expected CLOSE"))
		return messages.SubscriptionUUID(""), false
	}
	subscriptionHandle, ok := globalOngoingSubscriptions.Load(subscriptionToClose)
	if !ok {
		span.RecordError(fmt.Errorf("could not find subscription among ongoing"))
		return messages.SubscriptionUUID(""), false
	}
	err := subscriptionDB.DeleteOne(ctx, subscriptionHandle.Details)
	if err != nil {
		span.RecordError(fmt.Errorf("subscription DB delete error: %w", err))
	}
	subscriptionHandle.Cancel()
	globalOngoingSubscriptions.Delete(subscriptionToClose)
	return subscriptionHandle.Details.UUID(), true
}
