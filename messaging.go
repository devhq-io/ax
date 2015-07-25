package ax

import (
	"encoding/json"
	"log"
	"sync"
)

type JsonClientMessage struct {
	MsgType string `json:"type"`
	Data interface{} `json:"data"`
}

type JsonMessageHandler func (c *Client, data interface{})
type RawMessageHandler func (c *Client, data []byte) bool

var (
	msgMap = make(map[string]JsonMessageHandler)
	msgMutex sync.RWMutex
	rawHandler RawMessageHandler
)

func (c *Client) JsonSend(msgtype string, arg interface{}) error {
	msg := &JsonClientMessage {
		MsgType: msgtype,
		Data: arg,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("messaging.Send error %+v", err)
		return err
	}
	return c.Send(data)
}

func jsonInvalidMessage(c *Client, data []byte) {
	log.Printf("Recv invalid JSON message (%+v, %+v)\n", c, data)
}

func jsonHandleMessage(c *Client, msgtype string, data interface{}) bool {
	msgMutex.RLock()
	handler, exists := msgMap[msgtype]
	if !exists {
		msgMutex.RUnlock()
		return false
	}
	handler(c, data)
	msgMutex.RUnlock()
	return true
}

func jsonRecv(c *Client, data []byte) {
	var msg JsonClientMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		jsonInvalidMessage(c, data)
	} else {
		jsonHandleMessage(c, msg.MsgType, msg.Data)
	}
}

func rawRecv(c *Client, data []byte) bool {
	r := false
	msgMutex.RLock()
	if rawHandler != nil {
		r = rawHandler(c, data)
	}
	msgMutex.RUnlock()
	return r
}

// Entrypoint for incoming messages
func onRecv(c *Client, data []byte) {
	r := rawRecv(c, data)
	if !r {
		jsonRecv(c, data)
	}
}

func OnJson(msgtype string, handler JsonMessageHandler) {
	msgMutex.Lock()
	msgMap[msgtype] = handler
	msgMutex.Unlock()
}

func OnRaw(handler RawMessageHandler) {
	msgMutex.Lock()
	rawHandler = handler
	msgMutex.Unlock()
}
