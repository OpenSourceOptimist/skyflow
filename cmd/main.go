package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"nhooyr.io/websocket"
)

func main() {
	log.SetLevel(log.InfoLevel)
	mongoUri := os.Getenv("MONGODB_URI")
	log.Info(mongoUri)
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoUri))
	if err != nil {
		panic(err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()
	s := &store.Store{EventCol: client.Database("skyflow").Collection("events")}
	ctx := context.Background()
	for err != nil {
		err := client.Ping(ctx, nil)
		log.Error("mongo ping error: " + err.Error())
		time.Sleep(time.Second)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug("handling new request")
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Error("error setting up websocket", "error", err)
			return
		}
		log.Debug("request upgraded to websocket")
		defer c.Close(websocket.StatusInternalError, "thanks, bye")
		ctx := r.Context()
		ongoingSubscriptions := make(map[messages.SubscriptionID]context.CancelFunc)
		events, requests, closes, errs := messages.ListenForMessages(ctx, c)
		for {
			log.Debug("listening for messages")
			select {
			case <-ctx.Done():
				c.Close(websocket.StatusNormalClosure, "session cancelled")
				log.Info("closing websocket")
				return
			case e := <-events:
				log.Debug("recived event")
				err := s.Save(ctx, e)
				if err != nil {
					log.Error("saveing event", "error", err)
				}
			case req := <-requests:
				log.Debug("recived request")
				requestCtx, requestCancelled := context.WithCancel(ctx)
				defer requestCancelled()
				if _, ok := ongoingSubscriptions[req.ID]; ok {
					log.Error("request for ongoing supscription", "requestSubscriptionID", req.ID, "allSubsForSession", ongoingSubscriptions)
					continue
				}
				ongoingSubscriptions[req.ID] = requestCancelled
				foundEvents := s.Get(requestCtx, req.Filter)
				go WriteFoundEventsToConnection(requestCtx, req.ID, foundEvents, conn)
			case close := <-closes:
				log.Debug("recived close")
				cancelSubscriptionFunc, ok := ongoingSubscriptions[close.Subscription]
				if !ok {
					continue
				}
				cancelSubscriptionFunc()
			case err = <-errs:
				log.Debug("recived error")
				if err != nil {
					log.Error("listening for websocket messages: " + err.Error())
				}
				continue
			}
		}
	})
	log.Info("server stopping: " + http.ListenAndServe(":80", handler).Error())

func WriteFoundEventsToConnection(ctx context.Context, sub messages.SubscriptionID, foundEvents <-chan event.Event, connection *websocket.Conn) {
	for {
		select {
		case <-ctx.Done():
			logrus.Debug("request context cancelled", "subId", sub)
			return
		case e := <-foundEvents:
			logrus.Debug("got event on request channel: ", e.ID)
			eventMsg, err := json.Marshal([]any{"EVENT", sub, e})
			if err != nil {
				logrus.Debug("marshalling eventMsg to send", "subId", sub, "error", err)
				continue
			}
			err = connection.Write(ctx, websocket.MessageText, eventMsg)
			if err != nil {
				logrus.Error("request attemp writing to websocket", "error", err)
			}
			logrus.Debug("event written on channel: ", string(eventMsg))
		}
	}
}
