package store

import (
	"context"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/log"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/sirupsen/logrus"
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
		findOpts := options.
			Find().
			SetSort(primitive.D{{Key: "created_at", Value: -1}})
		if filter.Limit != 0 {
			findOpts.SetLimit(filter.Limit)
		}
		cursor, err := s.EventCol.Find(ctx, filter.AsMongoQuery(), findOpts)
		if err != nil {
			logrus.Error("Store.Get mongo find operation: ", err)
			return //TODO: exponential backoff?
		}
		for cursor.Next(ctx) && cursor.Err() == nil {
			var e event.Event
			//TODO: unclear how we should handle an error here
			err := cursor.Decode(&e)
			if err != nil {
				logrus.Error("decoding event from database: ", err)
			}

			logrus.Debug("Store.Get event from mongo: ", log.Marshall(e))
			select {
			case res <- e:
			case <-ctx.Done():
				return
			}
		}
		logrus.Debug("Store.Get: initial query completed: next: ", cursor.Next(ctx), ", error: ", cursor.Err())
	}()
	return res
}
