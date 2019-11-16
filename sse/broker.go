package sse

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	ContentType              = "Content-Type"
	CacheControl             = "Cache-Control"
	Connection               = "Connection"
	AccessControlAllowOrigin = "Access-Control-Allow-Origin"
)

const (
	TextEventStream = "text/event-stream"
	NoCache         = "no-cache"
	KeepAlive       = "keep-alive"
)

type Broker struct {
	Notifier       chan []byte
	newClients     chan chan []byte
	closingClients chan chan []byte
	clients        map[chan []byte]bool
}

func (broker *Broker) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	flusher, ok := rw.(http.Flusher)

	if !ok {
		http.Error(rw, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	rw.Header().Set(ContentType, TextEventStream)
	rw.Header().Set(CacheControl, NoCache)
	rw.Header().Set(Connection, KeepAlive)
	rw.Header().Set(AccessControlAllowOrigin, "*")

	messageChan := make(chan []byte)
	broker.newClients <- messageChan

	defer func() {
		broker.closingClients <- messageChan
	}()

	notify := req.Context().Done()

	go func() {
		<-notify
		broker.closingClients <- messageChan
	}()

	for {
		_, _ = fmt.Fprintf(rw, "data: %s\n\n", <-messageChan)
		flusher.Flush()
	}
}

func (broker *Broker) listen() {
	for {
		select {
		case s := <-broker.newClients:
			broker.clients[s] = true
			log.Printf("Client added. %d registered clients", len(broker.clients))
		case s := <-broker.closingClients:
			delete(broker.clients, s)
			log.Printf("Removed client. %d registered clients", len(broker.clients))
		case event := <-broker.Notifier:
			for clientMessageChan, _ := range broker.clients {
				clientMessageChan <- event
			}
		}
	}
}

func (broker *Broker) Event(writer http.ResponseWriter, request *http.Request) {
	eventString := fmt.Sprintf("the time is %v", time.Now())
	log.Println("Receiving event")
	broker.Notifier <- []byte(eventString)
}

func NewBroker() (broker *Broker) {
	broker = &Broker{
		Notifier:       make(chan []byte, 1),
		newClients:     make(chan chan []byte),
		closingClients: make(chan chan []byte),
		clients:        make(map[chan []byte]bool),
	}

	go broker.listen()

	return
}
