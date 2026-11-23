package probe

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"sync"
)

// tryServeHTTPPortal sniffs the first bytes of a directory connection. Plain
// HTTP is routed to the GM/registration handler so the account portal can share
// the one public port operators actually open; anything else is returned as a
// buffered connection for the native QQTang protocol.
func (server *Server) tryServeHTTPPortal(connection net.Conn) (net.Conn, bool) {
	if server.gmHandler == nil {
		return connection, false
	}
	reader := bufio.NewReaderSize(connection, 4096)
	head, err := reader.Peek(4)
	if err != nil {
		return &bufferedConn{Conn: connection, reader: reader}, false
	}
	method := string(head)
	if method != "GET " && method != "POST" && method != "HEAD" && method != "OPTI" {
		return &bufferedConn{Conn: connection, reader: reader}, false
	}
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }
	wrapped := &bufferedConn{Conn: connection, reader: reader, onClose: closeDone}
	http.Serve(&singleConnListener{conn: wrapped, done: done, closeDone: closeDone}, server.gmHandler)
	return nil, true
}

// bufferedConn keeps peeked bytes available to the native protocol reader.
type bufferedConn struct {
	net.Conn
	reader  *bufio.Reader
	onClose func()
}

func (conn *bufferedConn) Read(p []byte) (int, error) { return conn.reader.Read(p) }

func (conn *bufferedConn) Close() error {
	if conn.onClose != nil {
		conn.onClose()
	}
	return conn.Conn.Close()
}

// singleConnListener hands one connection to http.Serve and keeps Accept
// blocked until it closes, so Serve does not tear down the in-flight response.
type singleConnListener struct {
	conn      net.Conn
	done      chan struct{}
	closeDone func()
	served    bool
	mu        sync.Mutex
}

func (listener *singleConnListener) Accept() (net.Conn, error) {
	listener.mu.Lock()
	if listener.served {
		listener.mu.Unlock()
		<-listener.done
		return nil, io.EOF
	}
	listener.served = true
	listener.mu.Unlock()
	return listener.conn, nil
}

func (listener *singleConnListener) Close() error {
	if listener.closeDone != nil {
		listener.closeDone()
	}
	return nil
}

func (listener *singleConnListener) Addr() net.Addr { return listener.conn.LocalAddr() }
