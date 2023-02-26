package store

import (
	"context"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
)

type Store struct {
	data []event.Event
}

func (s *Store) Save(ctx context.Context, e event.Event) error {
	s.data = append(s.data, e)
	return nil
}

func (s *Store) Get(ctx context.Context, filter messages.RequestFilter) <-chan event.Event {
	res := make(chan event.Event)
	go func() {
		for _, e := range s.data {
			select {
			case <-ctx.Done():
				return
			case res <- e:
				continue
			}
		}
		s.data = nil
	}()
	return res
}
