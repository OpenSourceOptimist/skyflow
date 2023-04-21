package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
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
		rawWebsocketMessages := websocketMessages(ctx, conn, l)
		parsedWebSocketMessages := slice.MapChan(ctx, rawWebsocketMessages, messages.ParseWebsocketMsg)
		for msg := range parsedWebSocketMessages {
			l.IncrementSession()
			if msg.Err != nil {
				l.Error("error reading websocket message", "error", msg.Err)
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
				err = eventDatabase.InsertOne(ctx, event.Structure(e))
				if err != nil {
					l.Error("failed to insert event in DB", "error", err)
				}
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
				defer cancelSubscription()                                     //nolint:staticcheck
				if _, ok := globalOngoingSubscriptions.Load(subscription.UUID()); ok {
					continue
				}
				err = subscriptions.InsertOne(ctx, subscription)
				if err != nil {
					l.Error("failed to insert subscription in DB", "error", err)
				}
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
				dbEventsStructured := slice.Merge(subscriptionCtx, eventsPerFilter...)
				dbEvents := slice.MapChan(subscriptionCtx, dbEventsStructured, event.UnStructure)
				dbEventsAsMessages := slice.MapChanSkipErrors(
					ctx, dbEvents, eventToWebsocketMsg(subscription.ID))
				newEventsAsMessages := slice.MapChanSkipErrors(
					ctx, newEvents, eventToWebsocketMsg(subscription.ID))
				subscriptionEvents := slice.ChanConcatenate(
					dbEventsAsMessages,
					slice.AsClosedChan(eoseWebsocketMsg(subscription.ID)),
					newEventsAsMessages)
				go writeToConnection(subscriptionCtx, subscriptionEvents, conn)
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
				err = subscriptions.DeleteOne(ctx, subscriptionHandle.Details)
				if err != nil {
					l.Error("failed to delete subscription", "error", err)
				}
				subscriptionHandle.Cancel()
				globalOngoingSubscriptions.Delete(subscriptionToClose)
			}
		}
		l.Info("closing websocket")
		conn.Close(websocket.StatusNormalClosure, "session cancelled")
		for _, subscriptionID := range subscriptionsAssosiatedWithRequest {
			subscriptionHandler, ok := globalOngoingSubscriptions.Load(subscriptionID)
			if !ok {
				continue
			}
			subscriptionHandler.Cancel()
			globalOngoingSubscriptions.Delete(subscriptionID)
			err = subscriptions.DeleteOne(ctx, subscriptionHandler.Details)
			if err != nil {
				l.Error("failed to delete subscription", "error", err)
			}
		}
	})
	logrus.Info("server stopping: " + http.ListenAndServe(":80", handler).Error())
}

func eoseWebsocketMsg(sub messages.SubscriptionID) []byte {
	bytes, err := json.Marshal([]any{"EOSE", sub})
	if err != nil {
		panic("failed to marshal EOSE message")
	}
	return bytes
}

func writeToConnection(ctx context.Context, msgChan <-chan []byte, connection *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		case eventMsg := <-msgChan:
			_ = connection.Write(ctx, websocket.MessageText, eventMsg)
		}
	}
}

func eventToWebsocketMsg(sub messages.SubscriptionID) func(event.Event) ([]byte, error) {
	return func(e event.Event) ([]byte, error) {
		eventMsg, err := json.Marshal([]any{"EVENT", sub, e})
		if err != nil {
			return nil, err
		}
		return eventMsg, nil
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

func websocketMessages(ctx context.Context, r *websocket.Conn, l *log.Logger) <-chan []byte {
	result := make(chan []byte)
	go func() {
		var wg sync.WaitGroup
		defer func() {
			wg.Wait()
			close(result)
		}()
		for {
			err := ctx.Err()
			if err != nil {
				return
			}
			socketMsgType, data, err := r.Read(ctx)
			if err != nil {
				l.Debug("read error", "error", err)
				if strings.Contains(err.Error(), "WebSocket closed") {
					return
				}
				if strings.Contains(err.Error(), "connection reset by peer") {
					return
				}
				if strings.Contains(err.Error(), "StatusGoingAway") {
					return
				}
				if strings.Contains(err.Error(), "EOF") {
					return
				}
			} else if socketMsgType == websocket.MessageText {
				// You must always read from the
				// connection. Otherwise control frames will
				// not be handled. See Reader and CloseRead
				wg.Add(1)
				go func(d []byte) {
					result <- d
					wg.Done()
				}(data)
			}
		}
	}()
	return result
}
