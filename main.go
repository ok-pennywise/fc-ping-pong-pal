package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096}
var broker = NewBroker()

func connEndpoint(w http.ResponseWriter, r *http.Request) {
	channel := r.PathValue("channel")
	id := r.PathValue("id")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	broker.StartTrackingConn(channel, id, conn)
	defer broker.StopTrackingConn(channel, id, conn)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Println("Error receiving message")
			}
			return
		}
		// var m *Message
		// json.Unmarshal(message, &m)
		broker.rdb.Publish(context.Background(), channel, string(message))
	}
}

func main() {
	http.HandleFunc("/{id}/{channel}", connEndpoint)
	http.ListenAndServe(":8000", nil)
}
