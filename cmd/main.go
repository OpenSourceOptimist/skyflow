package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	// "github.com/nbd-wtf/go-nostr"
	"nhooyr.io/websocket"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			panic(err)
		}
		defer c.Close(websocket.StatusInternalError, "the sky is falling")

		ctx, cancel := context.WithTimeout(r.Context(), time.Second*10)
		defer cancel()

		msgType, data, err := c.Read(ctx)
		if err != nil {
			log.Print(err)
			return
		}

		if msgType != websocket.MessageText {
			log.Printf("unexpected message type: %d", msgType)
			return
		}

		var e event.Event
		err = json.Unmarshal(data, &e)
		if err != nil {
			log.Print(err)
			return
		}

		log.Printf("received: %v", e)

		c.Write(ctx, websocket.MessageText, data)

		c.Close(websocket.StatusNormalClosure, "")
	})

	http.ListenAndServe(":80", handler)
}
