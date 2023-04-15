package help

import (
	"context"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	nostr "github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/require"
)

type PubKey string

func (priv PubKey) String() string {
	return string(priv)
}

type PrivKey string

func (priv PrivKey) String() string {
	return string(priv)
}

type EventOptions struct {
	PrivKey   PrivKey
	CreatedAt time.Time
	Kind      int
	Tags      nostr.Tags
	Content   string
}

func NewSignedEvent(t require.TestingT, opts EventOptions) event.Event {
	privKey := opts.PrivKey
	if privKey == "" {
		privKey = PrivKey(nostr.GeneratePrivateKey())
	}
	pubKey, err := nostr.GetPublicKey(privKey.String())
	require.NoError(t, err, "generating pubkey")
	e := &nostr.Event{
		PubKey:    pubKey,
		CreatedAt: opts.CreatedAt,
		Kind:      opts.Kind,
		Tags:      opts.Tags,
		Content:   opts.Content,
	}
	require.NoError(t, e.Sign(privKey.String()), "signing test event")
	return ToEvent(*e)
}

func NewKeyPair(t *testing.T) (PrivKey, event.PubKey) {
	priv := nostr.GeneratePrivateKey()
	pub, err := nostr.GetPublicKey(priv)
	require.NoError(t, err)
	return PrivKey(priv), event.PubKey(pub)
}

func NewConnection(t *testing.T) *nostr.Relay {
	relay, err := nostr.RelayConnect(context.Background(), "ws://localhost:80")
	require.NoError(t, err)
	return relay
}
