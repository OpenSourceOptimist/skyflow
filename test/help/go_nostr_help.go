package help

import (
	"context"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/google/uuid"
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

func WithPrivKey(pk PrivKey) EventOptions {
	return EventOptions{PrivKey: pk}
}

func WithCreatedAt(createdAt time.Time) EventOptions {
	return EventOptions{CreatedAt: createdAt}
}
func WithKind(kind int) EventOptions {
	return EventOptions{Kind: kind}
}
func WithTags(tags nostr.Tags) EventOptions {
	return EventOptions{Tags: tags}
}

func WithContent(content string) EventOptions {
	return EventOptions{Content: content}
}

func Event(t require.TestingT, opts ...EventOptions) event.Event {
	var opt EventOptions
	for _, o := range opts {
		if o.PrivKey != "" {
			opt.PrivKey = o.PrivKey
		}
		var nilTime time.Time
		if o.CreatedAt != nilTime {
			opt.CreatedAt = o.CreatedAt
		}
		if o.Kind != 0 {
			opt.Kind = o.Kind
		}
		if o.Tags != nil {
			opt.Tags = o.Tags
		}
		if o.Content != "" {
			opt.Content = o.Content
		}
	}
	privKey := opt.PrivKey
	if privKey == "" {
		privKey = PrivKey(nostr.GeneratePrivateKey())
	}
	pubKey, err := nostr.GetPublicKey(privKey.String())
	require.NoError(t, err, "generating pubkey")
	if opt.Content == "" {
		opt.Content = uuid.NewString()
	}
	e := &nostr.Event{
		PubKey:    pubKey,
		CreatedAt: opt.CreatedAt,
		Kind:      opt.Kind,
		Tags:      opt.Tags,
		Content:   opt.Content,
	}
	require.NoError(t, e.Sign(privKey.String()), "signing test event")
	return toEvent(*e)
}

func EventWithPrivKey(t require.TestingT, pk PrivKey) event.Event {
	return Event(t, WithPrivKey(pk))
}
func EventWithCreatedAt(t require.TestingT, createdAt time.Time) event.Event {
	return Event(t, WithCreatedAt(createdAt))
}
func EventWithKind(t require.TestingT, kind int) event.Event {
	return Event(t, WithKind(kind))
}
func EventWithTags(t require.TestingT, tags nostr.Tags) event.Event {
	return Event(t, WithTags(tags))
}
func EventWithContent(t require.TestingT, content string) event.Event {
	return Event(t, WithContent(content))
}

func EventWithPTagReferenceTo(t require.TestingT, pubKey event.PubKey) event.Event {
	return EventWithTags(t, nostr.Tags{nostr.Tag{"p", string(pubKey), ""}})
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
