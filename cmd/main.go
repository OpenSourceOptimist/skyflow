package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	// "github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/store"

	// "github.com/nbd-wtf/go-nostr"
	"nhooyr.io/websocket"
)

func main() {

	s := &store.Store{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Println(err.Error())
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
				return
			case e := <-events:
				s.Save(ctx, e)
			case req := <-requests:
				requestCtx, requestCancelled := context.WithCancel(ctx)
				defer requestCancelled()
				if _, ok := ongoingSubscriptions[req.ID]; ok {
					continue
				}
				ongoingSubscriptions[req.ID] = requestCancelled
				requestChan := s.Get(requestCtx, req.Filter)
				go func() {
					for {
						select {
						case <-requestCtx.Done():
							return
						case e, ok := <-requestChan:
							if !ok {
								return
							}
							eventMsg, err := json.Marshal([]any{"EVENT", e})
							if err != nil {
								continue
							}
							c.Write(requestCtx, websocket.MessageText, eventMsg)
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
					fmt.Println(err.Error())
				}
				continue
			}
		}
	})
	http.ListenAndServe(":80", handler)
}
