package event

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

type EventID string // <32-bytes lowercase hex-encoded sha256 of the the serialized event data>
func (id EventID) Bytes() ([]byte, error) {
	return hex.DecodeString(string(id))
}

type Timestamp int64 // <unix timestamp in seconds>,
type EventKind int64

// ["e", <32-bytes hex of the id of another event>, <recommended relay URL>],
// ["p", <32-bytes hex of the key>, <recommended relay URL>],
type EventTag []string

type PubKey string //  <32-bytes lowercase hex-encoded public key of the event creator>
func (pk PubKey) BIP340Secp256k1() (btcec.PublicKey, error) {
	pubKeyBytes, err := hex.DecodeString(string(pk))
	if err != nil {
		return btcec.PublicKey{}, err
	}

	pubKey, err := schnorr.ParsePubKey(pubKeyBytes)
	if err != nil {
		return btcec.PublicKey{}, err
	} else if pubKey == nil {
		return btcec.PublicKey{}, fmt.Errorf("non-error nil pointer pubkey from tcec.ParsePubKey")
	}

	return *pubKey, nil
}

type EventSignature string // <64-bytes hex of the signature of the sha256 hash of the serialized event data, which is the same as the "id" field>

func (sig EventSignature) Hex() string {
	return string(sig)
}

func (sig EventSignature) Bytes() ([]byte, error) {
	return hex.DecodeString(sig.Hex())
}

func (sig EventSignature) BIP340Secp256k1() (schnorr.Signature, error) {
	sigBytes, err := sig.Bytes()
	if err != nil {
		return schnorr.Signature{}, err
	}

	signature, err := schnorr.ParseSignature(sigBytes)
	if err != nil {
		return schnorr.Signature{}, err
	} else if signature == nil {
		return schnorr.Signature{}, fmt.Errorf("non-error nil pointer pubkey from tcec.ParseSignature")
	}

	return *signature, nil
}

type Event struct {
	ID        EventID        `json:"id"`
	PubKey    PubKey         `json:"pubkey"`
	CreatedAt Timestamp      `json:"created_at"`
	Kind      EventKind      `json:"kind"`
	Tags      []EventTag     `json:"tags"`
	Content   string         `json:"content"`
	Sig       EventSignature `json:"sig"`
}

func Map[T any, K any](slice []T, f func(T) K) []K {
	res := make([]K, 0, len(slice))
	for _, t := range slice {
		res = append(res, f(t))
	}
	return res
}

func NewEventID(pub PubKey, createdAt Timestamp, kind EventKind, tags []EventTag, content string) (EventID, error) {
	bytes, err := json.Marshal([]interface{}{
		0,
		pub,
		createdAt,
		kind,
		tags,
		content,
	})
	if err != nil {
		return EventID(""), err
	}

	eventIDByteArray := sha256.Sum256(bytes)
	return EventID(hex.EncodeToString(eventIDByteArray[:])), nil
}

func VerifyEventID(e Event) error {
	id, err := NewEventID(e.PubKey, e.CreatedAt, e.Kind, e.Tags, e.Content)
	if err != nil {
		return err
	}
	if id != e.ID {
		return fmt.Errorf("event id verification failed")
	}
	return nil
}

func VerifySignature(e Event) error {
	pubKey, err := e.PubKey.BIP340Secp256k1()
	if err != nil {
		return fmt.Errorf("pubkey parsing: %w", err)
	}

	signature, err := e.Sig.BIP340Secp256k1()
	if err != nil {
		return fmt.Errorf("signature parsing: %w", err)
	}

	if err := VerifyEventID(e); err != nil {
		return fmt.Errorf("event id verification: %w", err)
	}

	eventIDBytes, err := e.ID.Bytes()
	if err != nil {
		return fmt.Errorf("event id to bytes: %w", err)
	}

	if !signature.Verify(eventIDBytes, &pubKey) {
		return fmt.Errorf("signature verification failed")
	}
	return nil

}
