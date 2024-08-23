package main

import (
	"context"
	"log"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type Broadcaster struct {
	redisConn *redis.Client

	mx *sync.Mutex
	wg *sync.WaitGroup

	sockets      map[string]map[string][]*websocket.Conn
	totalSockets int

	channelContexts map[string]*context.CancelFunc

	interComm chan *redis.Message
}

func NewBroadcaster() *Broadcaster {
	b := &Broadcaster{
		redisConn:       redis.NewClient(&redis.Options{Addr: "localhost:6379"}),
		sockets:         make(map[string]map[string][]*websocket.Conn),
		totalSockets:    0,
		channelContexts: make(map[string]*context.CancelFunc),
		interComm:       make(chan *redis.Message, 512),
		mx:              &sync.Mutex{},
		wg:              &sync.WaitGroup{},
	}
	b.startBroadcasting()
	return b
}

func (b *Broadcaster) StartTracking(channel, connId string, conn *websocket.Conn) {
	b.mx.Lock()
	defer b.mx.Unlock()

	if _, ok := b.sockets[channel]; !ok {
		b.sockets[channel] = make(map[string][]*websocket.Conn)
	}

	b.sockets[channel][connId] = append(b.sockets[channel][connId], conn)

	if _, ok := b.channelContexts[channel]; !ok {
		subscriber := b.redisConn.Subscribe(context.Background(), channel)
		ctx, cancel := context.WithCancel(context.Background())
		b.channelContexts[channel] = &cancel
		go b.readMessage(ctx, subscriber)
	}

	b.totalSockets += 1
	log.Printf("New client added. Client count: %d", b.totalSockets)
}

func (b *Broadcaster) StopTracking(channel, connId string, conn *websocket.Conn) {
	b.mx.Lock()
	defer b.mx.Unlock()

	if socketsByChannel, ok := b.sockets[channel]; ok {
		// Retrieve the slice of connections for the given connId
		sockets := socketsByChannel[connId]

		// Create a new slice to hold the remaining connections
		// Iterate over the existing slice and append only the connections that do not match the one being removed
		for i, socket := range sockets {
			if socket == conn {
				b.sockets[channel][connId] = slices.Delete(b.sockets[channel][connId], i, i+1)
				break
			}
		}

		if len(b.sockets[channel][connId]) == 0 {
			delete(b.sockets[channel], connId)
		}

		// Clean up the channel if there are no connections left
		if len(b.sockets[channel]) == 0 {
			delete(b.sockets, channel)
			if cancelGoRoutine, ok := b.channelContexts[channel]; ok {
				(*cancelGoRoutine)()
				delete(b.channelContexts, channel)
			}
		}
	}

	if b.totalSockets > 0 {
		b.totalSockets -= 1
	}

	log.Printf("New client removed. Client count: %d", b.totalSockets)
}

func (b *Broadcaster) SwitchChannel(connId, oldChannel, newChannel string) {
	b.mx.Lock()
	defer b.mx.Unlock()

	if sockets, ok := b.sockets[oldChannel][connId]; ok {
		delete(b.sockets[oldChannel], connId)

		if _, ok := b.sockets[newChannel]; !ok {
			b.sockets[newChannel] = make(map[string][]*websocket.Conn)
		}

		b.sockets[newChannel][connId] = append(b.sockets[newChannel][connId], sockets...)
	}
}

func (b *Broadcaster) readMessage(ctx context.Context, subscriber *redis.PubSub) {
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
			b.interComm <- message
		}
	}
}

func (b *Broadcaster) broadcastMessage() {
	for message := range b.interComm {
		b.mx.Lock()

		socketsByChannel, ok := b.sockets[message.Channel]
		if !ok {
			b.mx.Unlock()
			continue
		}

		// Create a wait group to wait for all go routines to finish

		// Split sockets into chunks
		socketsSlice := make([]*websocket.Conn, 0)
		for _, connList := range socketsByChannel {
			socketsSlice = append(socketsSlice, connList...)
		}

		batchSize := 100
		for i := 0; i < len(socketsSlice); i += batchSize {
			end := i + batchSize
			if end > len(socketsSlice) {
				end = len(socketsSlice)
			}

			// Launch a go routine for each batch
			b.wg.Add(1)
			go func(start, end int) {
				defer b.wg.Done()
				for _, socket := range socketsSlice[start:end] {
					if err := socket.WriteMessage(websocket.TextMessage, []byte(message.Payload)); err != nil {
						log.Println("Error sending message:", err)
					}
				}
			}(i, end)
		}

		b.mx.Unlock()
		b.wg.Wait() // Wait for all go routines to finish
	}
}

func (b *Broadcaster) startBroadcasting() {
	go b.broadcastMessage()
}
