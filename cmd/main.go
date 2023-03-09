package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/log"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"github.com/sirupsen/logrus"
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
	s := &store.Store{EventCol: client.Database("skyflow").Collection("events")}
	ctx := context.Background()
	for err != nil {
		err := client.Ping(ctx, nil)
		logrus.Error("mongo ping error: " + err.Error())
		time.Sleep(time.Second)
	}

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
		ongoingSubscriptions := make(map[messages.SubscriptionID]context.CancelFunc)
		events, requests, closes, errs := messages.ListenForMessages(ctx, conn)
		for {
			logrus.Debug("listening for messages")
			select {
			case <-ctx.Done():
				conn.Close(websocket.StatusNormalClosure, "session cancelled")
				logrus.Info("closing websocket")
				return
			case e := <-events:
				logrus.Debug("recived event: ", e.ID) //log.Marshall(e))
				err := s.Save(ctx, e)
				if err != nil {
					logrus.Error("saveing event", "error", err)
				}
			case req := <-requests:
				logrus.Debug("recived request: ", log.Marshall(req))
				requestCtx, requestCancelled := context.WithCancel(ctx)
				defer requestCancelled()
				if _, ok := ongoingSubscriptions[req.ID]; ok {
					logrus.Error("request for ongoing supscription", "requestSubscriptionID", req.ID, "allSubsForSession", ongoingSubscriptions)
					continue
				}
				ongoingSubscriptions[req.ID] = requestCancelled
				foundEvents := s.Get(requestCtx, req.Filter)
				go WriteFoundEventsToConnection(requestCtx, req.ID, foundEvents, conn)
			case close := <-closes:
				logrus.Debug("recived close: ", close.Subscription) //log.Marshall(close))
				cancelSubscriptionFunc, ok := ongoingSubscriptions[close.Subscription]
				if !ok {
					continue
				}
				cancelSubscriptionFunc()
			case err = <-errs:
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
