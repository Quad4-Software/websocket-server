package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512 * 1024 // 512KB

	// Channel buffer size for outgoing messages
	sendBufferSize = 256
)

// Server represents the WebSocket relay server.
type Server struct {
	clients     map[*websocket.Conn]*ClientInfo
	mutex       sync.RWMutex
	port        int
	host        string
	mode        string
	domain      string
	rateLimiter *rate.Limiter
	clientLimit int
	httpServer  *http.Server
}

// ClientInfo holds the state for an individual connected WebSocket client.
type ClientInfo struct {
	send      chan []byte
	limiter   *rate.Limiter
	closeOnce sync.Once
}

// NewServer creates a new instance of the WebSocket relay server.
func NewServer(host string, port int, mode string, domain string, rateLimit int, burst int, clientLimit int) *Server {
	return &Server{
		clients:     make(map[*websocket.Conn]*ClientInfo),
		port:        port,
		host:        host,
		mode:        mode,
		domain:      domain,
		rateLimiter: rate.NewLimiter(rate.Limit(rateLimit), burst),
		clientLimit: clientLimit,
	}
}

func (s *Server) addClient(conn *websocket.Conn) *ClientInfo {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.clientLimit > 0 && len(s.clients) >= s.clientLimit {
		return nil
	}

	info := &ClientInfo{
		send:    make(chan []byte, sendBufferSize),
		limiter: rate.NewLimiter(rate.Limit(100), 10),
	}
	s.clients[conn] = info
	log.Printf("Client connected. Total clients: %d", len(s.clients))
	return info
}

func (s *Server) removeClient(conn *websocket.Conn) {
	s.mutex.Lock()
	info, ok := s.clients[conn]
	if ok {
		delete(s.clients, conn)
		info.closeOnce.Do(func() {
			close(info.send)
		})
	}
	count := len(s.clients)
	s.mutex.Unlock()
	log.Printf("Client disconnected. Total clients: %d", count)
}

func (s *Server) broadcast(data []byte, exclude *websocket.Conn) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if len(data) > 0xFFFFFFFF-4 {
		return
	}

	packetLen := uint32(len(data)) // #nosec G115
	packet := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(packet[:4], packetLen)
	copy(packet[4:], data)

	for conn, info := range s.clients {
		if conn == exclude {
			continue
		}
		select {
		case info.send <- packet:
		default:
			// If buffer is full, drop connection or message.
			// For high-performance relay, we drop the message or the slow client.
			go s.removeClient(conn)
		}
	}
}

func (s *Server) writePump(conn *websocket.Conn, info *ClientInfo) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	for {
		select {
		case message, ok := <-info.send:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The server closed the channel.
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimiter.Allow() {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	if s.domain != "" {
		isWSS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		if !isWSS {
			http.Error(w, "WSS required", http.StatusBadRequest)
			return
		}
	}

	upgrader := s.getUpgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	clientInfo := s.addClient(conn)
	if clientInfo == nil {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Server at capacity"))
		_ = conn.Close()
		return
	}

	// Start write pump in a separate goroutine
	go s.writePump(conn, clientInfo)

	// Configure read settings
	conn.SetReadLimit(maxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	defer s.removeClient(conn)

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		if !clientInfo.limiter.Allow() {
			break
		}

		if messageType != websocket.BinaryMessage {
			continue
		}

		if len(data) < 4 {
			continue
		}

		packetLen := binary.BigEndian.Uint32(data[:4])
		if len(data) < int(packetLen)+4 {
			continue
		}

		packet := data[4 : 4+packetLen]

		switch s.mode {
		case "echo":
			s.echoPacket(clientInfo, packet)
		case "broadcast":
			s.broadcast(packet, conn)
		case "log":
			logLen := 32
			if len(packet) < logLen {
				logLen = len(packet)
			}
			log.Printf("Packet logged: %x", packet[:logLen])
		}
	}
}

func (s *Server) echoPacket(info *ClientInfo, packet []byte) {
	if len(packet) > 0xFFFFFFFF-4 {
		return
	}

	packetLen := uint32(len(packet)) // #nosec G115
	response := make([]byte, 4+len(packet))
	binary.BigEndian.PutUint32(response[:4], packetLen)
	copy(response[4:], packet)

	select {
	case info.send <- response:
	default:
		// Drop message if buffer full
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		r.URL.Path = "/index.html"
	}
	// Use path.Clean to prevent directory traversal
	// However, it's better to use a dedicated directory
	http.ServeFile(w, r, "public"+r.URL.Path)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// Start initializes and runs the HTTP/WebSocket server.
func (s *Server) Start() error {
	wsHandler := s.reverseProxyMiddleware(s.corsMiddleware(s.cspMiddleware(s.handleWebSocket)))
	staticHandler := s.reverseProxyMiddleware(s.forceWSSMiddleware(s.corsMiddleware(s.cspMiddleware(s.handleStatic))))
	healthHandler := s.handleHealth

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler)
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/", staticHandler)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	log.Printf("Starting WebSocket server on %s (mode: %s)", addr, s.mode)

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	s.mutex.Lock()
	for conn := range s.clients {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Server shutting down"))
		// We don't remove client here to avoid deadlock as removeClient also locks mutex
		// But we should close the send channel
		if info, ok := s.clients[conn]; ok {
			info.closeOnce.Do(func() {
				close(info.send)
			})
		}
	}
	s.mutex.Unlock()

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *Server) getUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
}
