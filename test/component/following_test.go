package component

import (
	"context"
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

func TestGetEventsAfterInitialSync(t *testing.T) {
	priv, pub := NewKeyPair(t)
	conn1 := NewConnection(t)
	ctx := context.Background()
	initialPublishedEvent := NewSignedEvent(t, EventOptions{PrivKey: priv, Content: "hello world"})
	status := conn1.Publish(ctx, initialPublishedEvent)
	require.Equal(t, nostr.PublishStatusSucceeded, status)

	conn2 := NewConnection(t)
	subscription := conn2.Subscribe(ctx, nostr.Filters{{Authors: []string{string(pub)}}})
	select {
	case initialRecivedEvent := <-subscription.Events:
		require.Equal(t, toEvent(initialPublishedEvent), toEvent(*initialRecivedEvent))
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for initial event to be recived")
	}

	secondPublishedEvent := NewSignedEvent(t, EventOptions{PrivKey: priv, Content: "hello again"})
	require.Equal(t,
		conn1.Publish(ctx, secondPublishedEvent),
		nostr.PublishStatusSucceeded,
	)

	select {
	case secondRecivedEvent := <-subscription.Events:
		require.Equal(t, toEvent(secondPublishedEvent), toEvent(*secondRecivedEvent))
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for second event to be recived")
	}
}
