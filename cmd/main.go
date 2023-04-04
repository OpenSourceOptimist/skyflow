package main

import (
	"context"
	"encoding/json"
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
	logrus.Info("Starting Skyflow")
	mongoUri := os.Getenv("MONGODB_URI")
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoUri))
	if err != nil {
		logrus.Fatal("Mongo connection failed")
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	logrus.Info("Mongo successfully connected")
	eventDatabase := &store.Store[event.StructuredEvent]{
		Col: client.Database("skyflow").Collection("events"),
	}
	subscriptions := &store.Store[messages.Subscription]{
		Col: client.Database("skyflow").Collection("subscriptions"),
	}
	ctx := context.Background()
	for err != nil {
		err := client.Ping(ctx, nil)
		logrus.Error("mongo ping", "error", err.Error())
		time.Sleep(time.Second)
	}

	globalOngoingSubscriptions := NewSyncMap[messages.SubscriptionUUID, SubscriptionHandle]()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := messages.SessionID(uuid.NewString())
		l := &log.Logger{Session: slice.Prefix(string(session), 5)}
		l.Debug("handling new request")
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			l.Error("error setting up websocket", "error", err)
			return
		}
		defer func() {
			l.Debug("websocket closed")
			conn.Close(websocket.StatusInternalError, "thanks, bye")
		}()
		ctx := r.Context()
		subscriptionsAssosiatedWithRequest := make([]messages.SubscriptionUUID, 0)
		websocketMessages := messages.ListenForMessages(ctx, conn)
		for {
			l.IncrementSession()
			var msg messages.WebsocketMessage
			select {
			case msg = <-websocketMessages:
			case <-ctx.Done():
				l.Info("ctx cancelled, closing websocket")
				conn.Close(websocket.StatusNormalClosure, "session cancelled by user")
				for _, subscriptionID := range subscriptionsAssosiatedWithRequest {
					subscriptionHandler, ok := globalOngoingSubscriptions.Load(subscriptionID)
					if !ok {
						continue
					}
					subscriptionHandler.Cancel()
					globalOngoingSubscriptions.Delete(subscriptionID)
					_ = subscriptions.DeleteOne(ctx, subscriptionHandler.Details)
				}
				return //nolint:govet
			}
			if msg.Err != nil {
				l.Error("error", err)
				continue
			}
			switch msg.MsgType {
			case messages.EVENT:
				e, ok := msg.AsEvent()
				if !ok {
					l.Error("not event", "msg", msg)
					continue
				}
				l.Debug("event over websocket", "eID", e.ID)
				_ = eventDatabase.InsertOne(ctx, event.Structure(e))
				for subscription := range subscriptions.Find(ctx, messages.SubscriptionFilter(e)) {
					handle, ok := globalOngoingSubscriptions.Load(subscription.UUID())
					if !ok {
						l.Error("subscription not in global map", "subID", subscription.ID)
						continue
					}
					l.Debug("following up", "subID", subscription.ID, "eID", e.ID)
					slice.AsyncWrite(handle.Ctx, handle.NewEvents, e)
				}
			case messages.REQ:
				subscription, ok := msg.AsREQ(session)
				if !ok {
					l.Error("not REQ", "msg", msg)
				}
				l.Debug("subscription over websocket", "subscription", subscription)
				subscriptionCtx, cancelSubscription := context.WithCancel(ctx) //nolint:govet
				defer cancelSubscription()
				if _, ok := globalOngoingSubscriptions.Load(subscription.UUID()); ok {
					continue
				}
				_ = subscriptions.InsertOne(ctx, subscription)
				newEvents := make(chan event.Event)
				globalOngoingSubscriptions.Store(
					subscription.UUID(),
					SubscriptionHandle{
						Ctx:       subscriptionCtx,
						Cancel:    cancelSubscription,
						NewEvents: newEvents,
						Details:   subscription,
					},
				)
				eventsPerFilter := slice.Map(subscription.Filters,
					func(filter messages.Filter) <-chan event.StructuredEvent {
						return eventDatabase.Find(
							subscriptionCtx,
							messages.EventFilter(filter),
							store.FindOptions{
								Sort:  primitive.D{{Key: "event.created_at", Value: -1}},
								Limit: filter.Limit,
							},
						)
					})
				eventsInDatabase := slice.Merge(subscriptionCtx, eventsPerFilter...)
				events := slice.MapChan(subscriptionCtx, eventsInDatabase, event.UnStructure)
				subscriptionEvents := slice.ChanConcatenate(events, newEvents)
				go writeFoundEventsToConnection(subscriptionCtx, subscription.ID, subscriptionEvents, conn)
			case messages.CLOSE:
				subscriptionToClose, ok := msg.AsCLOSE(session)
				if !ok {
					continue
				}
				l.Debug("recived close over websocket", "subID", subscriptionToClose)
				subscriptionHandle, ok := globalOngoingSubscriptions.Load(subscriptionToClose)
				if !ok {
					continue
				}
				subscriptionHandle.Cancel()
				globalOngoingSubscriptions.Delete(subscriptionToClose)
			}
		}
	})
	logrus.Info("server stopping: " + http.ListenAndServe(":80", handler).Error())
}

func writeFoundEventsToConnection(
	ctx context.Context, sub messages.SubscriptionID, foundEvents <-chan event.Event, connection *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-foundEvents:
			eventMsg, err := json.Marshal([]any{"EVENT", sub, e})
			if err != nil {
				continue
			}
			_ = connection.Write(ctx, websocket.MessageText, eventMsg)
		}
	}
}

type SubscriptionHandle struct {
	Ctx       context.Context
	Cancel    func()
	NewEvents chan<- event.Event
	Details   messages.Subscription
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
