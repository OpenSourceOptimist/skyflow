package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/log"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"nhooyr.io/websocket"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stdout)
	mongoUri := os.Getenv("MONGODB_URI")
	logrus.Info(mongoUri)
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoUri))
	if err != nil {
		panic(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	eventDatabase := &store.Store[event.Event]{
		Col: client.Database("skyflow").Collection("events"),
	}
	subscriptions := &store.Store[messages.Subscription]{
		Col: client.Database("skyflow").Collection("subscriptions"),
	}
	ctx := context.Background()
	for err != nil {
		err := client.Ping(ctx, nil)
		logrus.Error("mongo ping error: " + err.Error())
		time.Sleep(time.Second)
	}

	globalOngoingSubscriptions := NewSyncMap[messages.SubscriptionID, SubscriptionHandle]()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logrus.Debug("handling new request")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			logrus.Error("error setting up websocket", "error", err)
			return
		}
		logrus.Debug("request upgraded to websocket")
		defer func() {
			logrus.Debug("websocket closed")
			conn.Close(websocket.StatusInternalError, "thanks, bye")
		}()
		ctx := r.Context()
		subscriptionsAssosiatedWithRequest := make([]messages.SubscriptionID, 0)
		eventChan, subscriptionReqChan, subscriptionCloseChan, errChan := messages.ListenForMessages(ctx, conn)
		for {
			logrus.Debug("listening for messages")
			select {
			case <-ctx.Done():
				logrus.Info("ctx cancelled, closing websocket")
				conn.Close(websocket.StatusNormalClosure, "session cancelled by user")
				for _, subscriptionID := range subscriptionsAssosiatedWithRequest {
					subscription, ok := globalOngoingSubscriptions.Load(subscriptionID)
					if !ok {
						continue
					}
					subscription.cancel()
					globalOngoingSubscriptions.Delete(subscriptionID)
				}
				return //nolint:govet
			case e := <-eventChan:
				logrus.Debug("recived event: ", e.ID)
				_ = eventDatabase.InsertOne(ctx, e)
				for subscription := range subscriptions.Find(ctx, messages.SubscriptionFilter(e)) {
					handle, ok := globalOngoingSubscriptions.Load(subscription.ID)
					if !ok {
						logrus.Error("database contained subscription not in global map")
						continue
					}
					slice.AsyncWrite(handle.ctx, handle.newEvents, e)
				}
			case subscription := <-subscriptionReqChan:
				logrus.Debug("recived subscription: ", log.Marshall(subscription))
				// Things will be angry that we do fancy logic to the cancel function.
				subscriptionCtx, cancelSubscription := context.WithCancel(ctx) //nolint:govet
				// Be carefull with this when refactoring.
				defer cancelSubscription()
				if _, ok := globalOngoingSubscriptions.Load(subscription.ID); ok {
					// Ignore subscription ID duplicates for now
					continue
				}
				eventsInDatabase := eventDatabase.Find(
					subscriptionCtx,
					messages.EventFilter(subscription.Filter),
					store.FindOptions{
						Sort:  primitive.D{{Key: "created_at", Value: -1}},
						Limit: subscription.Filter.Limit,
					},
				)
				newEvents := make(chan event.Event)
				globalOngoingSubscriptions.Store(
					subscription.ID,
					SubscriptionHandle{
						ctx:       subscriptionCtx,
						cancel:    cancelSubscription,
						newEvents: newEvents,
					},
				)
				subscriptionEvents := slice.ChanConcatenate(eventsInDatabase, newEvents)
				go WriteFoundEventsToConnection(subscriptionCtx, subscription.ID, subscriptionEvents, conn)
			case close := <-subscriptionCloseChan:
				logrus.Debug("recived close: ", close.Subscription)
				subscriptionHandle, ok := globalOngoingSubscriptions.Load(close.Subscription)
				if !ok {
					continue
				}
				subscriptionHandle.cancel()
				globalOngoingSubscriptions.Delete(close.Subscription)
			case err = <-errChan:
				logrus.Debug("recived error: ", err)
				if err != nil {
					logrus.Error("listening for websocket messages: " + err.Error())
				}
				continue
			}
		}
	})
	logrus.Info("server stopping: " + http.ListenAndServe(":80", handler).Error())
}

func WriteFoundEventsToConnection(ctx context.Context, sub messages.SubscriptionID, foundEvents <-chan event.Event, connection *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			logrus.Debug("request context cancelled", "subId", sub)
			return
		case e := <-foundEvents:
			eventMsg, err := json.Marshal([]any{"EVENT", sub, e})
			if err != nil {
				logrus.Debug("marshalling eventMsg to send", "subId", sub, "error", err)
				continue
			}
			err = connection.Write(ctx, websocket.MessageText, eventMsg)
			if err != nil {
				logrus.Error("request attemp writing to websocket", "error", err)
			}
			logrus.Debug(fmt.Sprintf("eventID %s written to subscriptionID %s", e.ID, sub))
		}
	}
}

type SubscriptionHandle struct {
	ctx       context.Context
	cancel    func()
	newEvents chan<- event.Event
}

func NewSyncMap[K comparable, T any]() SyncMap[K, T] {
	return SyncMap[K, T]{syncMap: sync.Map{}}
}

type SyncMap[K comparable, T any] struct {
	syncMap sync.Map
}

func (m *SyncMap[K, T]) Delete(key K) {
	m.syncMap.Delete(key)
}

func (m *SyncMap[K, T]) Load(key K) (T, bool) {
	t, ok := m.syncMap.Load(key)
	if !ok {
		var nilT T
		return nilT, false
	}
	return t.(T), ok
}

func (m *SyncMap[K, T]) Store(key K, value T) {
	m.syncMap.Store(key, value)
}
