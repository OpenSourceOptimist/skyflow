package main

import (
	"context"
	"encoding/json"

	"net/http"

	"github.com/OpenSourceOptimist/skyflow/internal/log"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/store"
	"nhooyr.io/websocket"
)

func main() {

	s := &store.Store{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Error("error setting up websocket", "error", err)
			return
		}
		defer c.Close(websocket.StatusInternalError, "thanks, bye")
		ctx := r.Context()
		ongoingSubscriptions := make(map[messages.SubscriptionID]context.CancelFunc)
		events, requests, closes, errs := messages.ListenForMessages(ctx, c)
		for {
			select {
			case <-ctx.Done():
				c.Close(websocket.StatusNormalClosure, "session cancelled")
				log.Info("closing websocket")
				return
			case e := <-events:
				err := s.Save(ctx, e)
				if err != nil {
					log.Error("saveing event", "error", err)
				}
			case req := <-requests:
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
						case e, ok := <-requestChan:
							if !ok {
								log.Debug("request event chan closed", "subId", req.ID)
								return
							}
							eventMsg, err := json.Marshal([]any{"EVENT", e})
							if err != nil {
								log.Debug("marshalling eventMsg to send", "subId", req.ID, "error", err)
								continue
							}
							err = c.Write(requestCtx, websocket.MessageText, eventMsg)
							if err != nil {
								log.Error("writing to websocket", "error", err)
							}
						}
					}
				}()
			case close := <-closes:
				cancelSubscriptionFunc, ok := ongoingSubscriptions[close.Subscription]
				if !ok {
					continue
				}
				cancelSubscriptionFunc()
			case err = <-errs:
				if err != nil {
					log.Error("listening for websocket messages: " + err.Error())
				}
				continue
			}
		}
	})
	http.ListenAndServe(":80", handler)
}
