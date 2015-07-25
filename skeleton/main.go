package main

import (
	"time"
	"os"
	"io/ioutil"
	"bufio"
	"fmt"
	"log"
	"runtime/debug"
	"net/http"

	"github.com/devhq-io/ax"
	)

func setupLogger(which string) {
	switch which {
	case "null":
		log.SetOutput(ioutil.Discard)
	case "stdout":
		log.SetOutput(os.Stdout)
	case "file":
		f, err := os.Open("log.txt")
		if err != nil {
			panic(err)
		}
		defer func() {
			f.Close()
		} ()
		log.SetOutput(bufio.NewWriter(f))
	}
}

func startGcLoop(period time.Duration) {
	go func(period time.Duration) {
		for {
			log.Printf("gc\n")
			debug.FreeOSMemory()
			time.Sleep(period * time.Second)
		}
	} (period)
}

func indexFileHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./www/index.html")
}

func setupRoutes(r *ax.Router) {
	r.StrictSlash(true)
	r.HandleFunc("/", indexFileHandler)

	http.Handle("/", r)
}

func start(port int) {
	log.Printf("Serving %d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

var value int

func setMessageHandlers() {
	ax.OnJson("request",
		func (c *ax.Client, data interface{}) {
			type answerArgs struct {
				Value int `json:"value"`
			}
			value += 1
			log.Printf("request message: '%+v'\n", data)
			c.JsonSend("answer", &answerArgs{Value: value})
		})
}

func main() {
	port := 2000
	setupLogger("stdout")
	startGcLoop(1000)
	c := &ax.Config{Port: port, ConnectionTimeout: 300}
	r := ax.Setup(c)
	setupRoutes(r)
	setMessageHandlers()
	start(port)
}
