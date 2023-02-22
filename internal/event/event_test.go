package main

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewEventID(t *testing.T) {
	e := Event{
		ID:        EventID("d7dd5eb3ab747e16f8d0212d53032ea2a7cadef53837e5a6c66d42849fcb9027"),
		Kind:      EventKind(1),
		PubKey:    PubKey("22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c"),
		CreatedAt: Timestamp(1670869179),
		Content:   "NOSTR \"WINE-ACCOUNT\" WITH HARVEST DATE STAMPED\n\n\n\"The older the wine, the greater its reputation\"\n\n\n22a12a128a3be27cd7fb250cbe796e692896398dc1440ae3fa567812c8107c1c\n\n\nNWA 2022-12-12\nAA",
		Tags:      []EventTag{{"client", "astral"}},
		Sig:       EventSignature("f110e4fdf67835fb07abc72469933c40bdc7334615610cade9554bf00945a1cebf84f8d079ec325d26fefd76fe51cb589bdbe208ac9cdbd63351ddad24a57559"),
	}
	id, err := NewEventID(e.PubKey, e.CreatedAt, e.Kind, e.Tags, e.Content)
	require.NoError(t, err, "generating event")
	require.Equal(t, e.ID, id, "event id should match")

	require.NoError(t, VerifyEventID(e), "verifying event id")

	require.NoError(t, VerifySignature(e), "verifying signature")
}
