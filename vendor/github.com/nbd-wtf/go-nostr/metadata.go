package nostr

import (
	"encoding/json"
	"fmt"
)

type ProfileMetadata struct {
	Name    string `json:"name,omitempty"`
	About   string `json:"about,omitempty"`
	Picture string `json:"picture,omitempty"`
	NIP05   string `json:"nip05,omitempty"`
}

func ParseMetadata(event Event) (*ProfileMetadata, error) {
	if event.Kind != 0 {
		return nil, fmt.Errorf("event %s is kind %d, not 0", event.ID, event.Kind)
	}

	var meta ProfileMetadata
	err := json.Unmarshal([]byte(event.Content), &meta)
	if err != nil {
		cont := event.Content
		if len(cont) > 100 {
			cont = cont[0:99]
		}
		return nil, fmt.Errorf("failed to parse metadata (%s) from event %s: %w", cont, event.ID, err)
	}

	return &meta, nil
}
