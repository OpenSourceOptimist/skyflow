package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/handlers"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/rs/cors"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"nhooyr.io/websocket"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stdout)
	logrus.Info("Starting Skyflow")
	mongoUri := os.Getenv("MONGODB_URI")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exporter, err := otlptrace.New(ctx,
		otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint("tempo:4318"),
			otlptracehttp.WithInsecure(),
		),
	)
	if err != nil {
		logrus.Info("no tracing, you are flying blind")
	}
	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
	)
	defer func() { _ = traceProvider.Shutdown(context.Background()) }()
	otel.SetTracerProvider(traceProvider)
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoUri))
	if err != nil {
		logrus.Fatal("Mongo connection failed")
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	logrus.Info("Mongo successfully connected")
	eventDB := &store.Store[event.StructuredEvent]{
		Col: client.Database("skyflow").Collection("events"),
	}
	subscriptionDB := &store.Store[messages.Subscription]{
		Col: client.Database("skyflow").Collection("subscriptions"),
	}
	for err != nil {
		err := client.Ping(ctx, nil)
		logrus.Error("mongo ping", "error", err.Error())
		time.Sleep(time.Second)
	}

	globalSubscriptions := NewSyncMap[messages.SubscriptionUUID, messages.SubscriptionHandle]()

	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx, span := otel.Tracer("skyflow").Start(ctx, "session")
		defer span.End()
		logrus.Info("new connection: " + span.SpanContext().TraceID().String())
		session := messages.SessionID(uuid.NewString())
		span.SetAttributes(attribute.String("sessionId", string(session)))
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer func() {
			conn.Close(websocket.StatusInternalError, "thanks, bye")
		}()
		sessionSubscriptions := NewSet[messages.SubscriptionUUID]()
		rawWebsocketMessages := websocketMessages(ctx, conn)
		parsedWebSocketMessages := slice.MapChan(ctx, rawWebsocketMessages, messages.ParseWebsocketMsg)
		for msg := range parsedWebSocketMessages {
			if msg.Err != nil {
				continue
			}
			switch msg.MsgType {
			case messages.EVENT:
				handlers.Event(
					ctx, msg, eventDB, subscriptionDB, &globalSubscriptions,
				)
			case messages.REQ:
				subUUID, cancelFunc, ok := handlers.Req(
					ctx, session, msg, eventDB, subscriptionDB, &globalSubscriptions, conn,
				)
				defer cancelFunc() //nolint:staticcheck
				if ok {
					sessionSubscriptions.Add(subUUID)
				}
			case messages.CLOSE:
				subUUID, ok := handlers.Close(
					ctx, session, msg, subscriptionDB, &globalSubscriptions,
				)
				if ok {
					sessionSubscriptions.Remove(subUUID)
				}
			}
		}
		conn.Close(websocket.StatusNormalClosure, "session cancelled")
		for subscriptionUUID := range sessionSubscriptions {
			subscriptionHandler, ok := globalSubscriptions.Load(subscriptionUUID)
			if !ok {
				continue
			}
			subscriptionHandler.Cancel()
			globalSubscriptions.Delete(subscriptionUUID)
			err = subscriptionDB.DeleteOne(ctx, subscriptionHandler.Details)
			if err != nil {
				logrus.Error("mongo delete failed: " + err.Error())
			}
		}
	})
	handler = cors.AllowAll().Handler(handler)
	logrus.Info("server stopping: " + http.ListenAndServe(":80", handler).Error())
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

func NewSet[T comparable](values ...T) Set[T] {
	res := make(Set[T], len(values))
	for _, v := range values {
		res.Add(v)
	}
	return res
}

type Set[T comparable] map[T]struct{}

func (s *Set[T]) Add(t T) {
	if _, ok := (*s)[t]; !ok {
		(*s)[t] = struct{}{}
	}
}

func (s *Set[T]) Remove(t T) {
	delete((*s), t)
}

func websocketMessages(ctx context.Context, r *websocket.Conn) <-chan []byte {
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
