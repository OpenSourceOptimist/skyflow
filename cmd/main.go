package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/log"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"nhooyr.io/websocket"
)

func main() {
	logrus.SetLevel(logrus.InfoLevel)
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
				requestChan := s.Get(requestCtx, req.Filter)
				go func() {
					for {
						select {
						case <-requestCtx.Done():
							log.Debug("request context cancelled", "subId", req.ID)
							return
						case e := <-requestChan:
							log.Debug("request channel closed")
							eventMsg, err := json.Marshal([]any{"EVENT", e})
							if err != nil {
								log.Debug("marshalling eventMsg to send", "subId", req.ID, "error", err)
								continue
							}
							err = c.Write(requestCtx, websocket.MessageText, eventMsg)
							if err != nil {
								log.Error("request attemp writing to websocket", "error", err)
							}
						}
					}
				}()
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
}
