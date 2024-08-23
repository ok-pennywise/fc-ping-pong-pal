package main

import (
	"log"
	"net/http"
	"net/url"

	// "net/url"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}
var socketDirector = NewBroadcaster()

func echo(w http.ResponseWriter, r *http.Request) {

	connId := "632913"
	queryParams, err := url.ParseQuery(r.URL.RawQuery)

	if err != nil {
		http.Error(w, "Unable to parse query", http.StatusBadRequest)
		return
	}

	channel, ok := queryParams["channel"]

	if !ok {
		http.Error(w, "Must have a channel as a query param", http.StatusBadRequest)
		return
	}

	c, err := upgrader.Upgrade(w, r, nil)

	if err != nil {

		log.Print("Upgrade error:", err)
		return
	}
	defer c.Close()

	socketDirector.StartTracking(channel[0], connId, c)
	defer socketDirector.StopTracking(channel[0], connId, c)

	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			log.Println("Read message error:", err)
			break
		}
	}
}

func main() {
	http.HandleFunc("/", echo)
	log.Fatal(http.ListenAndServe(":8000", nil))

}
