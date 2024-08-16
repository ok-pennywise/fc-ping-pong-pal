package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}
var socketDirector = NewSocketManager()

var secretKey string

func verifyKey(keyString string) (*jwt.Token, error) {
	key, err := jwt.Parse(keyString, func(k *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})

	if err != nil {
		return nil, err
	}

	if !key.Valid {
		return nil, fmt.Errorf("Invalid token")
	}

	return key, nil
}

func generateRandomID() string {
	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())

	// Generate a random integer
	randomInt := rand.Intn(1000000) // Generates a random integer between 0 and 999999

	// Convert the integer to a string
	randomID := strconv.Itoa(randomInt)

	return randomID
}

func echo(w http.ResponseWriter, r *http.Request) {

	c, err := upgrader.Upgrade(w, r, nil)

	if err != nil {

		log.Print("Upgrade error:", err)
		return
	}
	defer c.Close()

	connId :=  "632913"
	socketDirector.Track("sf", connId,c)
	defer socketDirector.Untrack("sf", connId, c)

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
