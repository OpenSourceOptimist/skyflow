package component

import (
	"testing"
	"time"

	nostr "github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

type EventOptions struct {
	PrivKey   string
	CreatedAt time.Time
	Kind      int
	Tags      nostr.Tags
	Content   string
}

func NewSignedEvent(t *testing.T, opts EventOptions) nostr.Event {
	privKey := opts.PrivKey
	if privKey == "" {
		privKey = nostr.GeneratePrivateKey()
	}
	pubKey, err := nostr.GetPublicKey(privKey)
	require.NoError(t, err, "generating pubkey")
	e := &nostr.Event{
		PubKey:    pubKey,
		CreatedAt: opts.CreatedAt,
		Kind:      opts.Kind,
		Tags:      opts.Tags,
		Content:   opts.Content,
	}
	require.NoError(t, e.Sign(privKey), "signing test event")
	return *e
}
