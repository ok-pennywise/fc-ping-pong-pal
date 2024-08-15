package main

import (
	"context"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type SocketDirector struct {
	redisConn       *redis.Client
	mx              sync.Mutex
	sockets         map[string][]*websocket.Conn
	interComm       chan *redis.Message
	channelContexts map[string]*context.CancelFunc
}

func NewSocketDirector() *SocketDirector {

	sd := &SocketDirector{
		sockets:         make(map[string][]*websocket.Conn),
		redisConn:       redis.NewClient(&redis.Options{Addr: "localhost:6379"}),
		interComm:       make(chan *redis.Message, 1024),
		channelContexts: make(map[string]*context.CancelFunc),
	}

	// Start message broadcasting goroutine
	sd.startBroadcasting()

	return sd
}

func (sd *SocketDirector) Track(channel string, conn *websocket.Conn) {
	sd.mx.Lock()
	defer sd.mx.Unlock()

	// Add the new WebSocket connection to the channel
	sd.sockets[channel] = append(sd.sockets[channel], conn)

	// Subscribe to Redis channel if not already subscribed
	if _, ok := sd.channelContexts[channel]; !ok {
		subscriber := sd.redisConn.Subscribe(context.Background(), channel)
		ctx, cancel := context.WithCancel(context.Background())
		sd.channelContexts[channel] = &cancel
		go sd.readMessage(ctx, subscriber)
	}
}

func (sd *SocketDirector) Untrack(channel string, conn *websocket.Conn) {
	sd.mx.Lock()
	defer sd.mx.Unlock()

	// Remove the WebSocket connection from the channel
	for i, socket := range sd.sockets[channel] {
		if socket == conn {
			sd.sockets[channel] = append(sd.sockets[channel][:i], sd.sockets[channel][i+1:]...)
			break
		}
	}

	// Cleanup channel if there are no more connections
	if len(sd.sockets[channel]) == 0 {
		delete(sd.sockets, channel)
		if cancelFunc, ok := sd.channelContexts[channel]; ok {
			(*cancelFunc)()
			delete(sd.channelContexts, channel)
		}
	}
}

func (sd *SocketDirector) readMessage(ctx context.Context, subscriber *redis.PubSub) {
	for {
		select {
		case <-ctx.Done():
			subscriber.Close()
			return
		default:
			message, err := subscriber.ReceiveMessage(context.Background())
			if err != nil {
				log.Println("Error receiving message:", err)
				return
			}
			sd.interComm <- message
		}
	}
}

func (sd *SocketDirector) broadcastMessage() {
	for message := range sd.interComm {
		sd.mx.Lock()
		for _, socket := range sd.sockets[message.Channel] {
			err := socket.WriteMessage(websocket.TextMessage, []byte(message.Payload))
			if err != nil {
				log.Println("Error broadcasting message:", err)
				socket.Close()
				// sd.Untrack(message.Channel, socket)
			}
		}
		sd.mx.Unlock()
	}
}

func (sd *SocketDirector) startBroadcasting() {
	go sd.broadcastMessage()
}
