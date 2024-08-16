package main

import (
	"context"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type SocketManager struct {
	redisConn *redis.Client
	mx        sync.Mutex

	sockets         map[string]map[string][]*websocket.Conn
	channelContexts map[string]*context.CancelFunc

	interComm chan *redis.Message
}

func NewSocketManager() *SocketManager {
	sd := &SocketManager{
		redisConn:       redis.NewClient(&redis.Options{Addr: "localhost:6379"}),
		sockets:         make(map[string]map[string][]*websocket.Conn),
		channelContexts: make(map[string]*context.CancelFunc),
		interComm:       make(chan *redis.Message, 1024),
	}
	sd.startBroadcasting()
	return sd
}

func (sd *SocketManager) Track(channel, connId string, conn *websocket.Conn) {
	sd.mx.Lock()
	defer sd.mx.Unlock()

	if _, ok := sd.sockets[channel]; !ok {
		sd.sockets[channel] = make(map[string][]*websocket.Conn)
	}

	sd.sockets[channel][connId] = append(sd.sockets[channel][connId], conn)

	if _, ok := sd.channelContexts[channel]; !ok {
		subscriber := sd.redisConn.Subscribe(context.Background(), channel)
		ctx, cancel := context.WithCancel(context.Background())
		sd.channelContexts[channel] = &cancel
		go sd.readMessage(ctx, subscriber)
	}

	log.Println(sd.sockets)
}

func (sd *SocketManager) Untrack(channel, connId string, conn *websocket.Conn) {
	sd.mx.Lock()
	defer sd.mx.Unlock()

	if socketsByChannel, ok := sd.sockets[channel]; ok {
		// Retrieve the slice of connections for the given connId
		sockets := socketsByChannel[connId]

		// Create a new slice to hold the remaining connections
		// Iterate over the existing slice and append only the connections that do not match the one being removed
		for i, socket := range sockets {
			if socket == conn {
				sd.sockets[channel][connId] = append(sockets[:i], sockets[i+1:]...)
			}
		}

		if len(sd.sockets[channel][connId]) == 0 {
			delete(sd.sockets[channel], connId)
		}

		// Clean up the channel if there are no connections left
		if len(sd.sockets[channel]) == 0 {
			delete(sd.sockets, channel)
			if cancelFunc, ok := sd.channelContexts[channel]; ok {
				(*cancelFunc)()
				delete(sd.channelContexts, channel)
			}
		}
	}

	log.Println(sd.sockets)
}

func (sd *SocketManager) readMessage(ctx context.Context, subscriber *redis.PubSub) {
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

func (sd *SocketManager) broadcastMessage() {
	for message := range sd.interComm {
		sd.mx.Lock()
		for _, sockets := range sd.sockets[message.Channel] {
			if len(sockets) > 1 {
				for _, socket := range sockets {
					err := socket.WriteMessage(websocket.TextMessage, []byte(message.Payload))
					if err != nil {
						log.Println("Error sending message:", err)
					}
				}
			} else {
				sockets[0].WriteMessage(websocket.TextMessage, []byte(message.Payload))
			}

		}
		sd.mx.Unlock()
	}
}

func (sd *SocketManager) startBroadcasting() {
	go sd.broadcastMessage()
}
