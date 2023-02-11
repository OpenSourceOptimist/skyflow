package main

type NoteID string   // <32-bytes lowercase hex-encoded sha256 of the the serialized event data>
type PubKey string   //  <32-bytes lowercase hex-encoded public key of the event creator>
type Timestamp int64 // <unix timestamp in seconds>,
type EventKind int64

// ["e", <32-bytes hex of the id of another event>, <recommended relay URL>],
// ["p", <32-bytes hex of the key>, <recommended relay URL>],
type EventTag []string

type EventSignature string // <64-bytes hex of the signature of the sha256 hash of the serialized event data, which is the same as the "id" field>

type Note struct {
	ID        NoteID         `json:"id"`
	PubKey    string         `json:"pubkey"`
	CreatedAt Timestamp      `json:"created_at"`
	Kind      EventKind      `json:"kind"`
	Tags      []EventTag     `json:"tags"`
	Content   string         `json:"content"`
	Sig       EventSignature `json:"sig"`
}
