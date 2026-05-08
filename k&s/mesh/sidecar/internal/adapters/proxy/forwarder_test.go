package proxy

import (
	"net"
	"testing"
	"time"
)

type closeWriteConn struct {
	net.Conn
	closedWrite bool
}

func (c *closeWriteConn) CloseWrite() error {
	c.closedWrite = true
	return nil
}

func TestCloseWriteUsesCloseWriterInterface(t *testing.T) {
	conn := &closeWriteConn{}

	closeWrite(conn)

	if !conn.closedWrite {
		t.Fatal("expected CloseWrite to be called")
	}
}

func TestCloseWriteIgnoresConnectionsWithoutHalfClose(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := make(chan struct{})
	go func() {
		closeWrite(client)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closeWrite blocked on connection without CloseWrite")
	}
}
