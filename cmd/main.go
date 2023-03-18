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
	"github.com/google/uuid"
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
		log.Error("mongo ping", "error", err)
		time.Sleep(time.Second)
	}

	globalOngoingSubscriptions := NewSyncMap[messages.SubscriptionID, SubscriptionHandle]()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := uuid.NewString()[:5]
		log.Debug("handling new request", "session", session)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Error("error setting up websocket", "error", err, "session", session)
			return
		}
		defer func() {
			log.Debug("websocket closed", "session", session)
			conn.Close(websocket.StatusInternalError, "thanks, bye")
		}()
		ctx := r.Context()
		subscriptionsAssosiatedWithRequest := make([]messages.SubscriptionID, 0)
		eventChan, subscriptionReqChan, subscriptionCloseChan, errChan := messages.ListenForMessages(ctx, conn)
		for {
			select {
			case <-ctx.Done():
				log.Info("ctx cancelled, closing websocket", "session", session)
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
				log.Debug("recived event", "eID", e.ID, "session", session)
				_ = eventDatabase.InsertOne(ctx, e)
				subCount := 0
				for subscription := range subscriptions.Find(ctx, messages.SubscriptionFilter(e)) {
					subCount++
					handle, ok := globalOngoingSubscriptions.Load(subscription.ID)
					if !ok {
						log.Error("subscription not in global map",
							"subID", subscription.ID, "session", session)
						continue
					}
					log.Debug("following up",
						"subID", subscription.ID, "eID", e.ID, "session", session)
					slice.AsyncWrite(handle.ctx, handle.newEvents, e)
				}
				log.Debug("event to subscriptions", "eID", e.ID, "count", subCount)
			case subscription := <-subscriptionReqChan:
				log.Debug("recived subscription: ", "subscription", subscription)
				subscriptionCtx, cancelSubscription := context.WithCancel(ctx) //nolint:govet
				defer cancelSubscription()
				if _, ok := globalOngoingSubscriptions.Load(subscription.ID); ok {
					continue
				}
				_ = subscriptions.InsertOne(ctx, subscription)
				newEvents := make(chan event.Event)
				globalOngoingSubscriptions.Store(
					subscription.ID,
					SubscriptionHandle{
						ctx:       subscriptionCtx,
						cancel:    cancelSubscription,
						newEvents: newEvents,
					},
				)
				eventsInDatabase := eventDatabase.Find(
					subscriptionCtx,
					messages.EventFilter(subscription),
					store.FindOptions{
						Sort:  primitive.D{{Key: "created_at", Value: -1}},
						Limit: subscription.Limit,
					},
				)
				subscriptionEvents := slice.ChanConcatenate(eventsInDatabase, newEvents)
				go writeFoundEventsToConnection(subscriptionCtx, subscription.ID, subscriptionEvents, conn)
			case close := <-subscriptionCloseChan:
				log.Debug("recived close", "subID", close.Subscription)
				subscriptionHandle, ok := globalOngoingSubscriptions.Load(close.Subscription)
				if !ok {
					continue
				}
				subscriptionHandle.cancel()
				globalOngoingSubscriptions.Delete(close.Subscription)
			case err = <-errChan:
				log.Debug("recived error", "error", err)
				if err != nil {
					log.Error("websocket error", "error", err)
				}
				continue
			}
		}
	})
	log.Info("server stopping", "error", http.ListenAndServe(":80", handler).Error())
}

func writeFoundEventsToConnection(
	ctx context.Context, sub messages.SubscriptionID, foundEvents <-chan event.Event, connection *websocket.Conn) {
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
