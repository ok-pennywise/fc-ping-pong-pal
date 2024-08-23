package main

import (
	"fmt"
	"log"
	"time"
	"github.com/gorilla/websocket"
)

// StressTestConfig holds configuration for the stress test.
type StressTestConfig struct {
	URLs        []string
	NumClients   int
	MessageCount int
	Message      string
}

func main() {
	config := StressTestConfig{
		URLs:        []string{"ws://localhost:8000/?channel=sf"}, // Replace with your WebSocket server URL
		NumClients:   1000, // Number of WebSocket connections
	}

	for _, url := range config.URLs {
		for i := 0; i < config.NumClients; i++ {
			go startClient(url)
		}
	}

	// Wait for clients to finish
	time.Sleep(10 * time.Minute)
}

// startClient connects to the WebSocket server and sends messages.
func startClient(url string) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatalf("Failed to connect to WebSocket server: %v", err)
		return
	}
	defer conn.Close()

	for{
		_, message, err := conn.ReadMessage()
		if err != nil{
			fmt.Println("Error")
			return
		}
		fmt.Println("Received message %s", message)
	}

	fmt.Println("Started a client")

	time.Sleep(2 * time.Minute)
}
