package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}
var socketDirector = NewSocketDirector()

func echo(w http.ResponseWriter, r *http.Request) {

	if 1 == 1{
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	c, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		
		log.Print("Upgrade error:", err)
		return
	}
	defer c.Close()

	socketDirector.Track("sf", c)
	defer socketDirector.Untrack("sf", c)

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
