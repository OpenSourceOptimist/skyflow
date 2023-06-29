package handlers

import (
	"context"

	"github.com/OpenSourceOptimist/skyflow/internal/messages"
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
) {
	subscriptionToClose, ok := msg.AsCLOSE(session)
	if !ok {
		return
	}
	subscriptionHandle, ok := globalOngoingSubscriptions.Load(subscriptionToClose)
	if !ok {
		return
	}
	err := subscriptionDB.DeleteOne(ctx, subscriptionHandle.Details)
	if err != nil {
		return
	}
	subscriptionHandle.Cancel()
	globalOngoingSubscriptions.Delete(subscriptionToClose)
}
