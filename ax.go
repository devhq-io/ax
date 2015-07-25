package ax

import (
	"io/ioutil"
	"net/http"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/dchest/uniuri"
	"fmt"
	"log"
	"time"
	"io"
	"strings"
	"errors"
)

type Config struct {
	Port int
	UseTls bool
	ConnectionTimeout int
}

type Router struct {
	mux.Router
}

// Structure `Client` defines client contiguous connection.
type Client struct {
	// The WebSocket connection
	ws *websocket.Conn

	// Bufered channel of outbound messages
	send chan []byte

	// Channel for shutting down
	shutdown chan bool

	// Connection ID
	cid string

	t int64

	// User context (available for client code)
	Context map[string] interface{}
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 5 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 10 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 1) / 2

	// Maximum message size allowed from peer.
	maxMessageSize = 512 * 1024

	purgePeriod = 120 * time.Second
)

var (
	config Config
	upgrader = websocket.Upgrader {
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	onenter func(*Client, *http.Request)
	onleave func(*Client)
	onping func(*Client)
	ErrDisconnected = errors.New("Attempt to perform send/shutdown at disconnected client")
)

const cidCookieName = "__cid__"

func (c *Client) Cid() string {
	return c.cid
}

func genConnId() string {
	return uniuri.NewLen(20)
}

func getCurrentCid(r *http.Request) (string, error) {
	cookie, err := r.Cookie(cidCookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func makeCookie(cid string) *http.Cookie {
	expire := time.Now().Add(
		time.Duration(config.ConnectionTimeout) * time.Second)
	cookie := &http.Cookie {
		Name: cidCookieName,
		Value: cid,
		Path: "/",
		Expires: expire,
	}
	return cookie
}

func makeInitScript(cid string, host string, port int, usetls bool, connectionTimeout int) string {
	return fmt.Sprintf("var __state = {cid:'%s',conn_timeout:%d,host:'%s',port:%d,secure:%v};\n",
			   cid, connectionTimeout, host, port, usetls)
}

func getHostFromRequest(r *http.Request) string {
	s := strings.Split(r.Host, ":")
	if len(s) == 0 {
		log.Printf("Error: empty host in request\n")
		return ""
	} else {
		return s[0]
	}
}

func axInitHandler(w http.ResponseWriter, r *http.Request) {
	cid, err := getCurrentCid(r)
	if err != nil {
		cid = genConnId()
		http.SetCookie(w, makeCookie(cid))
	}
	w.Header().Set("Content-Type", "text/javascript")
	script := makeInitScript(cid, getHostFromRequest(r),
				 config.Port, config.UseTls, config.ConnectionTimeout)
	fmt.Fprintf(w, "%s", script)
}

func axStaticHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript")
	http.ServeFile(w, r, "./ax/ax.js");
}

func (c *Client) write(msgtype int, data []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(msgtype, data)
}

func sendLoop(c *Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[%v]sendLoop panic: %v\n", c.cid, r)
		}
		close(c.send)
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case <-c.shutdown:
			c.write(websocket.CloseMessage, []byte{})
			return
		case data, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.write(websocket.TextMessage, data);
								err != nil {
				log.Printf("[%v]ws.write TextMessage error %+v\n", c.cid, err)
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{});
								err != nil {
				log.Printf("[%v]ws.write PingMessage error %+v\n", c.cid, err)
				return
			}
			if onping != nil {
				onping(c)
			}
			// Refresh cookie's "expires" property to avoid cookie invalidation
			if time.Now().Unix() - c.t > int64(config.ConnectionTimeout / 2) {
				c.t = time.Now().Unix()
				c.setCidCookie()
			}
		}
	}
}

func recvLoop(c *Client) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[%v]recvLoop panic: %v\n", c.cid, r)
		}
		if c.Shutdown() == nil && onleave != nil {
			onleave(c)
		}
	}()
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for !c.isDisconnected() {
		_, r, err := c.ws.NextReader()
		if err != nil {
			if err != io.EOF &&
			   err.Error() != "websocket: close 1006 unexpected EOF" {
				log.Printf("[%v]ws.NextReader error %+v\n", c.cid, err)
			}
			break
		}
		data, err := ioutil.ReadAll(r)
		if err != nil {
			log.Printf("[%v]ws.ReadAll error %+v\n", c.cid, err)
			break
		}
		onRecv(c, data)
	}
}

func axWebsocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade error %+v\n", err)
		return
	}

	cid, err := getCurrentCid(r)
	if err != nil {
		cid = genConnId()
		log.Printf("ERROR no CID cookie in websocket handler\n")
		log.Printf("ERROR context will not be preserved on " +
			"page reload\n")
	}

	c := &Client {
		ws: conn,
		send: make(chan []byte, 256),
		shutdown: make(chan bool),
		cid: cid,
		t: time.Now().Unix(),
		Context: make(map[string]interface{}),
	}

	c.setCidCookie()

	if onenter != nil {
		onenter(c, r)
	}

	go sendLoop(c)

	recvLoop(c)
}

func Setup(c *Config) *Router {
	config = *c
	// Initialize routing
	r := mux.NewRouter()
	http.HandleFunc("/__ax_init.js", axInitHandler)
	http.HandleFunc("/__ax.js", axStaticHandler)
	http.HandleFunc("/__ws", axWebsocketHandler)
	r.PathPrefix("/static").Handler(http.FileServer(http.Dir(".")))
	return &Router{*r}
}

func OnEnter(handler func(c *Client, r *http.Request)) {
	onenter = handler
}

func OnLeave(handler func(c *Client)) {
	onleave = handler
}

func OnPing(handler func(c * Client)) {
	onping = handler
}

func (c *Client) setCidCookie() {
	c.Send([]byte(`{"type": "__ax_set_cookie", "data": {}}`))
}

func (c *Client) isDisconnected() bool { return c.t == -1; }

func (c *Client) Send(data []byte) error {
	if c.isDisconnected() {
		return ErrDisconnected
	}
	c.send <- data
	return nil
}

func (c *Client) Shutdown() error {
	if c.isDisconnected() {
		return ErrDisconnected
	}
	c.t = -1
	c.shutdown <- true
	close(c.shutdown)
	return nil
}

