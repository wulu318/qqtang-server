package main

// qqt-ws-gateway bridges a browser WebSocket to the native QQ堂 TCP game
// server. The wire format inside WebSocket binary frames is exactly the same
// byte stream the desktop client sends, so the JS client can reuse the original
// protocol code.
//
// It is intentionally a raw pipe: no protocol parsing, no authentication
// decisions. The game server remains authoritative.

import (
	"flag"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 64 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8090", "WebSocket listen address")
	target := flag.String("target", "127.0.0.1:18000", "game server TCP address")
	static := flag.String("static", "", "optional directory to serve at /")
	flag.Parse()

	if *static != "" {
		http.Handle("/", http.FileServer(http.Dir(*static)))
	}

	http.HandleFunc("/game", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrade: %v", err)
			return
		}
		defer ws.Close()

		conn, err := net.DialTimeout("tcp", *target, 5*time.Second)
		if err != nil {
			log.Printf("dial %s: %v", *target, err)
			return
		}
		defer conn.Close()

		var once sync.Once
		closeBoth := func() {
			once.Do(func() {
				ws.Close()
				conn.Close()
			})
		}
		defer closeBoth()

		// WebSocket -> TCP
		go func() {
			defer closeBoth()
			for {
				kind, data, err := ws.ReadMessage()
				if err != nil {
					return
				}
				if kind != websocket.BinaryMessage {
					continue
				}
				if _, err = conn.Write(data); err != nil {
					return
				}
			}
		}()

		// TCP -> WebSocket
		buffer := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buffer)
			if n > 0 {
				if writeErr := ws.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	})

	log.Printf("ws gateway listening on %s -> %s", *listen, *target)
	if err := http.ListenAndServe(*listen, nil); err != nil {
		log.Fatal(err)
	}
}
