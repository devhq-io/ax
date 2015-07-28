# ax

Ax is a lightweight Go web toolkit for creating single page applications. It is built on the top of Gorilla WebSocket library. The design is focused on simplicity and performance.

This is not a web framework. This is a utilitarian tool to solve practical problems. There are no routing, templates, rendering, databases etc. Only a WebSocket connection between frontend and backend for message passing.


## Quick start:

```bash
go get github.com/devhq-io/ax
cd  $GOPATH/src/github.com/devhq/ax
./axnew -n foo /tmp/foo
cd /tmp/foo
go build
./foo
```

Then navigate to http://localhost:2000

## JavaScript API (frontend)

Connecting to the backend:

```javascript
ax.connect(function () {
    // ... connected OK ...
});
```
Handling disconnects:
```javascript
ax.onDisconnect(function () {
    // ... disconnected ...
});
```

Sending a JSON message to the backend:
```javascript
ax.send('foo_msg', {foo_data: 'hello!'});
```

Handling JSON messages from the backend:
```javascript
ax.on('foo_msg_from_backend', function (data) {
	console.log('JSON message from the backend:' + JSON.stringify(data));
});
```

Handling raw messages (in your custom format) from the backend:
```javascript
ax.onRaw(function (data) {
	console.log('Raw message from the backend');
});
```

Note, if raw message handler have been set, JSON messages from the backend are ignored.

## Go API (backend)

Initialization:

```golang
c := &ax.Config{
        UseTls: true, // whether the client should use 'wss://' prefix instead of 'ws://'
        ConnectionTimeout: 300, // Time (seconds) during which client's
	                        // "connection ID" cookie is active
}
ax.Setup(c)
```

Then you need to set up the routing and start your HTTP(S) server. See skeleton/main.go for the example.

Handling new client entered:
```golang
ax.OnEnter(func(c *ax.Client, r *http.Request) {
	log.Printf("A new client with connection id '%s'\n", c.Cid())
})
```

Handling client leaving:
```golang
ax.OnLeave(func(c *ax.Client) {
	log.Printf("The client with connection id '%s' has left\n", c.Cid())
})
```

The client has a continuous WebSocket connection with the server. When it is closed at the client, OnLeave
callback is called.

The client has an unique connection ID string. It can be obtained as follows:

```golang
cid := c.Cid()
```

The client has a context in-memory map which can be easily used by the developer:

```golang
ax.OnEnter(func(c *ax.Client, r *http.Request) {
	c.Context["user_name"] = "John"
	c.Context["score"] = 123
}

...

score, ok := c.Context["score"].(int)
userName, ok := c.Context["user_name"].(string)
```

Handling JSON messages from the client's frontend:
```golang
ax.OnJson("foo_msg", func(c *ax.Client, data interface{}){
	log.Printf("'foo_msg' JSON message from client '%s': %+v\n", c.Cid(), data)
})
```

Handling raw messages from the client's frontend:
```golang
ax.OnRaw(func(c *ax.Client, data []byte) bool {
	log.Printf("Raw message from client '%s': %+v\n", c.Cid(), data)
	// return true if message has been handled, false otherwise
	return true
})
```

Sending a JSON message:
```golang
c.JsonSend("foo_msg_from_backend",
	&struct{
		UserName string `json:"user_name"`
		Score int `json:"score"`
	}{
		"John",
		123
	}
)
```

Sending a raw message:
```golang
c.Send([]byte("this is a raw binary message"))
```

Disconnecting the client (onDisconnect() will be called at the client):
```golang
c.Disconnect()
```

