package main

import (
	"context"
	"log"
	"net/http"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type Broadcaster struct {
	redisClient *redis.Client

	mx *sync.Mutex

	messageChannel chan *redis.Message

	sockets        map[string]map[string][]*websocket.Conn
	channelContext map[string]context.CancelFunc
}

func NewBroadcaster() *Broadcaster {
	b := &Broadcaster{
		redisClient:    redis.NewClient(&redis.Options{Addr: "localhost:6379"}), // Default Redis port is 6379
		mx:             &sync.Mutex{},
		messageChannel: make(chan *redis.Message, 1024),
		sockets:        make(map[string]map[string][]*websocket.Conn),
		channelContext: make(map[string]context.CancelFunc),
	}
	go b.broadcast()
	return b
}

func (b *Broadcaster) StartTracking(channel, connId string, conn *websocket.Conn) {
	b.mx.Lock()
	defer b.mx.Unlock()

	if _, ok := b.sockets[channel]; !ok {
		b.sockets[channel] = make(map[string][]*websocket.Conn)
	}
	b.sockets[channel][connId] = append(b.sockets[channel][connId], conn)

	if _, ok := b.channelContext[channel]; !ok {
		ctx, cancel := context.WithCancel(context.Background())
		b.channelContext[channel] = cancel
		subscriber := b.redisClient.Subscribe(ctx, channel)
		go b.readMessage(ctx, subscriber)
	}
	log.Println("", b.sockets)
}

func (b *Broadcaster) StopTracking(channel, connId string, conn *websocket.Conn) {
	b.mx.Lock()
	defer b.mx.Unlock()

	if conns, ok := b.sockets[channel][connId]; ok {
		for i, socket := range conns {
			if socket == conn {
				b.sockets[channel][connId] = slices.Delete(conns, i, i+1)
				break
			}
		}
	}

	if len(b.sockets[channel][connId]) == 0 {
		delete(b.sockets[channel], connId)
	}

	if len(b.sockets[channel]) == 0 {
		delete(b.sockets, channel)
		if cancelFunc, ok := b.channelContext[channel]; ok {
			cancelFunc()                      // Cancel the context to stop the goroutine
			delete(b.channelContext, channel) // Clean up the map
		}
	}

	log.Println("", b.sockets)
}

func (b *Broadcaster) readMessage(ctx context.Context, subscriber *redis.PubSub) {
	ch := subscriber.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Println("Context canceled, stopping readMessage")
			return
		case message, ok := <-ch:
			if !ok {
				log.Println("Channel closed")
				return
			}
			select {
			case b.messageChannel <- message:
			case <-ctx.Done():
				log.Println("Context canceled during message send")
				return
			}
		}
	}
}

func (b *Broadcaster) broadcast() {
	for message := range b.messageChannel {
		b.mx.Lock()
		socketsByChannel, ok := b.sockets[message.Channel]
		b.mx.Unlock()
		if !ok {
			continue
		}
		for _, socketsByConnId := range socketsByChannel {
			for _, socket := range socketsByConnId {
				err := socket.WriteMessage(websocket.TextMessage, []byte(message.Payload))
				if err != nil {
					log.Println("Failed to send message to WebSocket:", err)
				}
			}
		}
	}
}

var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
var broadcaster = NewBroadcaster()

func echo(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Unable to upgrade")
		return
	}
	defer conn.Close()

	id := "314323"
	broadcaster.StartTracking("sf", id, conn)
	defer broadcaster.StopTracking("sf", id, conn)

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Println("Error reading message:", err)
			}
			break
		}
	}
}

func main() {
	http.HandleFunc("/", echo)
	http.ListenAndServe(":8000", nil)
}
