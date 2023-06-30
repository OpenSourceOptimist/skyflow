package store

import (
	"context"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
)

type Collection interface {
	Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error)
	InsertOne(
		ctx context.Context,
		document interface{},
		opts ...*options.InsertOneOptions,
	) (*mongo.InsertOneResult, error)
	DeleteOne(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error)
}

type HasUniqueID interface {
	UniqueMatch() primitive.M
}

type Store[T HasUniqueID] struct {
	Col Collection
}

func (s *Store[T]) InsertOne(ctx context.Context, e T) error {
	ctx, span := otel.Tracer("skyflow").Start(ctx, "store_InsertOne")
	defer span.End()
	_, err := s.Col.InsertOne(ctx, e)
	return err
}

type FindOptions struct {
	Sort  primitive.D
	Limit int64
}

func (s *Store[T]) Find(ctx context.Context, filter primitive.M, opts ...FindOptions) <-chan T {
	ctx, span := otel.Tracer("skyflow").Start(ctx, "store_Find")
	defer span.End()
	findOpts := options.Find()
	for _, opt := range opts {
		if opt.Sort != nil {
			findOpts.SetSort(opt.Sort)
		}
		if opt.Limit != 0 {
			findOpts.SetLimit(opt.Limit)
		}
	}
	cursor, err := s.Col.Find(ctx, filter, findOpts)
	if err != nil {
		emptyChan := make(chan T)
		close(emptyChan)
		return emptyChan
	}
	return asChannel[T](ctx, cursor)
}

func (s *Store[T]) DeleteOne(ctx context.Context, t T) error {
	ctx, span := otel.Tracer("skyflow").Start(ctx, "store_DeleteOne")
	defer span.End()
	_, err := s.Col.DeleteOne(ctx, t.UniqueMatch())
	return err
}

func asChannel[T any](ctx context.Context, cursor *mongo.Cursor) <-chan T {
	res := make(chan T)
	go func() {
		defer close(res)
		for cursor.Next(ctx) && cursor.Err() == nil {
			var t T
			err := cursor.Decode(&t)
			if err != nil {
				return
			}
			select {
			case res <- t:
			case <-ctx.Done():
				return
			}
		}
	}()
	return res
}
