package component

import (
	"testing"
	"time"

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

func NewSignedEvent(t *testing.T, opts EventOptions) nostr.Event {
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
	return *e
}

func NewKeyPair(t *testing.T) (PrivKey, PubKey) {
	priv := nostr.GeneratePrivateKey()
	pub, err := nostr.GetPublicKey(priv)
	require.NoError(t, err)
	return PrivKey(priv), PubKey(pub)
}
