package protocol

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

type wifiTestServer struct {
	ln      net.Listener
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	handler func(Message) Message
}

func startWiFiTestServer(t *testing.T, handler func(Message) Message) *wifiTestServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	srv := &wifiTestServer{
		ln:      ln,
		ctx:     ctx,
		cancel:  cancel,
		handler: handler,
	}

	srv.wg.Add(1)
	go srv.acceptLoop()

	t.Cleanup(srv.Close)
	return srv
}

func (s *wifiTestServer) Addr() string {
	return s.ln.Addr().String()
}

func (s *wifiTestServer) Close() {
	s.cancel()
	_ = s.ln.Close()
	s.wg.Wait()
}

func (s *wifiTestServer) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			continue
		}

		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *wifiTestServer) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	msg, err := DecodeMessage(conn)
	if err != nil {
		return
	}

	resp := s.handler(msg)
	payload, err := EncodeMessage(resp)
	if err != nil {
		return
	}
	_, _ = conn.Write(payload)
}

func TestWiFiTransportIntegrationPing(t *testing.T) {
	received := make(chan Message, 1)
	srv := startWiFiTestServer(t, func(msg Message) Message {
		received <- msg
		return Message{
			ID:   msg.ID,
			Cmd:  msg.Cmd,
			Data: "pong",
		}
	})

	transport := &WiFiTransport{
		Addr:    srv.Addr(),
		Timeout: 1 * time.Second,
	}

	msg := Message{
		ID:  42,
		Cmd: "ping",
	}
	resp, err := transport.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.Data != "pong" {
		t.Fatalf("unexpected response data: %q", resp.Data)
	}

	select {
	case got := <-received:
		if got.ID != msg.ID || got.Cmd != msg.Cmd {
			t.Fatalf("unexpected request: %+v", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("server did not receive request")
	}
}

func TestWiFiTransportIntegrationMultipleRequests(t *testing.T) {
	received := make(chan Message, 2)
	srv := startWiFiTestServer(t, func(msg Message) Message {
		received <- msg
		return Message{
			ID:   msg.ID,
			Cmd:  msg.Cmd,
			Data: "ack:" + msg.Data,
		}
	})

	transport := &WiFiTransport{
		Addr:    srv.Addr(),
		Timeout: 1 * time.Second,
	}

	msg1 := Message{ID: 1, Cmd: "echo", Data: "first"}
	msg2 := Message{ID: 2, Cmd: "echo", Data: "second"}

	resp1, err := transport.Send(context.Background(), msg1)
	if err != nil {
		t.Fatalf("send first: %v", err)
	}
	if resp1.Data != "ack:first" {
		t.Fatalf("unexpected first response: %q", resp1.Data)
	}

	resp2, err := transport.Send(context.Background(), msg2)
	if err != nil {
		t.Fatalf("send second: %v", err)
	}
	if resp2.Data != "ack:second" {
		t.Fatalf("unexpected second response: %q", resp2.Data)
	}

	got := []Message{<-received, <-received}
	firstOK := got[0].ID == msg1.ID || got[1].ID == msg1.ID
	secondOK := got[0].ID == msg2.ID || got[1].ID == msg2.ID
	if !firstOK || !secondOK {
		t.Fatalf("unexpected requests: %+v", got)
	}
}
