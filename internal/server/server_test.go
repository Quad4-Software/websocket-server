package server

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHealthCheck(t *testing.T) {
	s := NewServer("localhost", 0, "echo", "", 10, 20, 100)
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	s.handleHealth(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expected := "OK"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
	}
}

func TestEchoMode(t *testing.T) {
	s := NewServer("127.0.0.1", 0, "echo", "", 100, 100, 100)

	server := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer ws.Close()

	message := []byte("hello world")
	packetLen := uint32(len(message))
	packet := make([]byte, 4+len(message))
	binary.BigEndian.PutUint32(packet[:4], packetLen)
	copy(packet[4:], message)

	if err := ws.WriteMessage(websocket.BinaryMessage, packet); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	_, p, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(p) < 4 {
		t.Fatalf("Packet too short: %d", len(p))
	}

	respLen := binary.BigEndian.Uint32(p[:4])
	if respLen != packetLen {
		t.Errorf("Wrong response length: got %d want %d", respLen, packetLen)
	}

	if string(p[4:]) != string(message) {
		t.Errorf("Wrong message: got %s want %s", p[4:], message)
	}
}

func TestBroadcastMode(t *testing.T) {
	s := NewServer("127.0.0.1", 0, "broadcast", "", 100, 100, 100)

	server := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	defer server.Close()

	u := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	ws1, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("Dial 1 failed: %v", err)
	}
	defer ws1.Close()

	ws2, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("Dial 2 failed: %v", err)
	}
	defer ws2.Close()

	// Give it a moment to register clients
	time.Sleep(100 * time.Millisecond)

	message := []byte("broadcast message")
	packetLen := uint32(len(message))
	packet := make([]byte, 4+len(message))
	binary.BigEndian.PutUint32(packet[:4], packetLen)
	copy(packet[4:], message)

	if err := ws1.WriteMessage(websocket.BinaryMessage, packet); err != nil {
		t.Fatalf("Write 1 failed: %v", err)
	}

	// ws2 should receive the message
	_, p, err := ws2.ReadMessage()
	if err != nil {
		t.Fatalf("Read 2 failed: %v", err)
	}

	if string(p[4:]) != string(message) {
		t.Errorf("Wrong message on ws2: got %s want %s", p[4:], message)
	}

	// ws1 should NOT receive its own message in broadcast mode
	ws1.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err = ws1.ReadMessage()
	if err == nil {
		t.Errorf("ws1 received its own message in broadcast mode")
	}
}
