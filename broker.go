package main

import (
	"context"
	"encoding/json"
	"log"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type Message struct {
	ID        string `json:"id"`         // Unique identifier for the message
	Version   int    `json:"version"`    // Version of the message format
	Channel   string `json:"channel"`    // Channel to which the message belongs
	EventType string `json:"event_type"` // Type of event (e.g., "update", "delete")

	Sender     string   `json:"sender"`     // Sender of the message
	Recipients []string `json:"recipients"` // List of recipients of the message

	Store bool                   `json:"store"` // Flag to determine if the message should be stored
	Data  map[string]interface{} `json:"data"`  // Additional data associated with the message
}

type Broker struct {
	rdb *redis.Client
	mx  *sync.RWMutex

	Conns map[string]map[string][]*websocket.Conn

	LegalChannels   []string
	channelContexts map[string]*context.CancelFunc
	Intercom        chan *Message
}

func NewBroker() *Broker {
	return &Broker{
		rdb:             redis.NewClient(&redis.Options{Addr: "localhost:6379"}),
		mx:              &sync.RWMutex{},
		Conns:           make(map[string]map[string][]*websocket.Conn),
		LegalChannels:   make([]string, 0),
		channelContexts: make(map[string]*context.CancelFunc),
		Intercom:        make(chan *Message, 512),
	}
}

func (b *Broker) StartTrackingConn(channel, connId string, conn *websocket.Conn) {
	b.mx.Lock()
	defer b.mx.Unlock()

	if _, ok := b.Conns[channel]; !ok {
		b.Conns[channel] = make(map[string][]*websocket.Conn)
	}

	b.Conns[channel][connId] = append(b.Conns[channel][connId], conn)

	if _, ok := b.channelContexts[channel]; !ok {
		ctx, cancelFunc := context.WithCancel(context.Background())
		b.channelContexts[channel] = &cancelFunc

		subscriber := b.rdb.Subscribe(ctx, channel)
		go b.ReadMessage(ctx, subscriber)
		go b.BroadcastMessage(ctx, channel)
	}
}

func (b *Broker) StopTrackingConn(channel, connId string, conn *websocket.Conn) {
	b.mx.Lock()
	defer b.mx.Unlock()

	conns := b.Conns[channel][connId]

	for i, c := range conns {
		if c == conn {
			b.Conns[channel][connId] = slices.Delete(b.Conns[channel][connId], i, i+1)
			break
		}
	}

	if len(b.Conns[channel][connId]) == 0 {
		delete(b.Conns[channel], connId)
	}

	if len(b.Conns[channel]) == 0 {
		delete(b.Conns, channel)
		cancelFunc := b.channelContexts[channel]
		(*cancelFunc)()
	}
}

func (b *Broker) ReadMessage(ctx context.Context, subscriber *redis.PubSub) {
	ch := subscriber.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Println("Unsubscribed")
			subscriber.Unsubscribe(ctx)
			return
		case message, ok := <-ch:
			if !ok {
				log.Println("Channel closed")
				return
			}
			var m *Message
			json.Unmarshal([]byte(message.Payload), &m)
			b.Intercom <- m
		}
	}
}

func (b *Broker) BroadcastMessage(ctx context.Context, channel string) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-b.Intercom:
			b.mx.RLock()
			connsByChannel := b.Conns[msg.Channel]
			b.mx.RUnlock()
			
			log.Println(connsByChannel)
			for _, recipient := range msg.Recipients {
				if conns, ok := connsByChannel[recipient]; ok {
					for _, conn := range conns {
						if err := conn.WriteJSON(msg); err != nil {
							log.Printf("Error sending message to WebSocket: %v", err)
							// Handle disconnection if needed
						}
					}
				}
			}
		}
	}
}
