package log

import (
	"testing"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/stretchr/testify/require"
)

func TestLogShouldNotPanic(t *testing.T) {
	l := &Logger{}
	t.Run("subscriptionID", func(t *testing.T) {
		require.NotPanics(t, func() {
			l.Debug("", "sub", messages.SubscriptionID("12"))
		})
	})
	t.Run("eventID", func(t *testing.T) {
		require.NotPanics(t, func() {
			l.Debug("", "event", event.ID("123"))
		})
	})
}
