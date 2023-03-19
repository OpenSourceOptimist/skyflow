package event

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

type ID string // <32-bytes lowercase hex-encoded sha256 of the the serialized event data>
func (id ID) Bytes() ([]byte, error) {
	return hex.DecodeString(string(id))
}

type Timestamp int64 // <unix timestamp in seconds>,
type Kind int64

// ["e", <32-bytes hex of the id of another event>, <recommended relay URL>],
// ["p", <32-bytes hex of the key>, <recommended relay URL>],
type Tag []string
type URL string

func E(t Tag) (ID, bool) {
	var nilID ID
	if len(t) != 3 {
		return nilID, false
	}
	if t[0] != "e" {
		return nilID, false
	}
	return ID(t[1]), true
}

func P(t Tag) (PubKey, bool) {
	var nilKey PubKey
	if len(t) != 3 {
		return nilKey, false
	}
	if t[0] != "p" {
		return nilKey, false
	}
	return PubKey(t[1]), true
}

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

type Signature string // <64-bytes hex of the signature of the sha256 hash of the serialized event data, which is the same as the "id" field>

func (sig Signature) Hex() string {
	return string(sig)
}

func (sig Signature) Bytes() ([]byte, error) {
	return hex.DecodeString(sig.Hex())
}

func (sig Signature) BIP340Secp256k1() (schnorr.Signature, error) {
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
	ID        ID        `json:"id" bson:"id"`
	PubKey    PubKey    `json:"pubkey" bson:"pubkey"`
	CreatedAt Timestamp `json:"created_at" bson:"created_at"`
	Kind      Kind      `json:"kind" bson:"kind"`
	Tags      []Tag     `json:"tags" bson:"tags"`
	Content   string    `json:"content" bson:"content"`
	Sig       Signature `json:"sig" bson:"sig"`
}

func Structure(e Event) StructuredEvent {
	return StructuredEvent{Event: e}
}

func UnStructure(e StructuredEvent) Event {
	return e.Event
}

type StructuredEvent struct {
	Event Event `bson:"event"`
	//E     []ID     `bson:"#e"`
	//P     []PubKey `bson:"#e"`
}

func (e StructuredEvent) UniqueID() string {
	return string(e.Event.ID)
}
func (e StructuredEvent) UniqueIDFieldName(format string) (string, error) {
	if format == "json" || format == "bson" {
		return "event.id", nil
	} else if format == "struct" {
		return "Event.ID", nil
	}
	return "", fmt.Errorf("only support format json, bson, and struct, not " + format)
}

func NewEventID(pub PubKey, createdAt Timestamp, kind Kind, tags []Tag, content string) (ID, error) {
	bytes, err := json.Marshal([]interface{}{
		0,
		pub,
		createdAt,
		kind,
		tags,
		content,
	})
	if err != nil {
		return ID(""), err
	}

	eventIDByteArray := sha256.Sum256(bytes)
	return ID(hex.EncodeToString(eventIDByteArray[:])), nil
}

func VerifyEvent(e Event) error {
	err := VerifyEventID(e)
	if err != nil {
		return fmt.Errorf("event id verification failed: %w", err)
	}
	err = VerifySignature(e)
	if err != nil {
		return fmt.Errorf("event signature verification failed: %w", err)
	}
	return nil
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

	eventIDBytes, err := e.ID.Bytes()
	if err != nil {
		return fmt.Errorf("event id to bytes: %w", err)
	}

	if !signature.Verify(eventIDBytes, &pubKey) {
		return fmt.Errorf("signature verification failed")
	}
	return nil

}
