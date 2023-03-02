package store

import (
	"context"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Collection interface {
	Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error)
	InsertOne(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error)
}

type Store struct {
	EventCol Collection
}

func (s *Store) Save(ctx context.Context, e event.Event) error {
	_, err := s.EventCol.InsertOne(ctx, e)
	return err
}

func (s *Store) Get(ctx context.Context, filter messages.RequestFilter) <-chan event.Event {
	res := make(chan event.Event)
	go func() {
		cursor, err := s.EventCol.Find(ctx, primitive.M{
			"$or": primitive.A{
				primitive.M{"id": primitive.M{"$in": filter.IDs}},
			},
		})
		if err != nil {
			return //TODO: exponential backoff
		}
		for cursor.Next(ctx) && cursor.Err() == nil {
			var e event.Event
			//TODO: unclear how we should handle an error here
			_ = cursor.Decode(&e)
			select {
			case res <- e:
			case <-ctx.Done():
				return
			}
		}
		// TODO: how to handle if next is false or we have a cursor error
	}()
	return res
}
