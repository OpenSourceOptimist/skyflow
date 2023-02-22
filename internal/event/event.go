package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type EventID string  // <32-bytes lowercase hex-encoded sha256 of the the serialized event data>
type PubKey string   //  <32-bytes lowercase hex-encoded public key of the event creator>
type Timestamp int64 // <unix timestamp in seconds>,
type EventKind int64

// ["e", <32-bytes hex of the id of another event>, <recommended relay URL>],
// ["p", <32-bytes hex of the key>, <recommended relay URL>],
type EventTag []string

type EventSignature string // <64-bytes hex of the signature of the sha256 hash of the serialized event data, which is the same as the "id" field>

type Event struct {
	ID        EventID        `json:"id"`
	PubKey    PubKey         `json:"pubkey"`
	CreatedAt Timestamp      `json:"created_at"`
	Kind      EventKind      `json:"kind"`
	Tags      []EventTag     `json:"tags"`
	Content   string         `json:"content"`
	Sig       EventSignature `json:"sig"`
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

func VerifyEventID(e Event) (bool, error) {
	id, err := NewEventID(e.PubKey, e.CreatedAt, e.Kind, e.Tags, e.Content)
	if err != nil {
		return false, err
	}
	return id == e.ID, nil
}
