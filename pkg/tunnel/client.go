package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	tunnelReconnectTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_tunnel_agent_reconnect_total",
		Help: "Total agent reconnection attempts",
	})
)

type AuthTokenVerifier func(context.Context, string, string, string) (time.Time, error)

type TokenSource interface {
	Token(context.Context) (string, error)
}

type TunnelServer struct {
	registry    *AgentRegistry
	verifyToken AuthTokenVerifier
	upgrader    websocket.Upgrader
	mu          sync.Mutex
}

func NewTunnelServer(verifyToken AuthTokenVerifier) *TunnelServer {
	return &TunnelServer{
		registry:    NewAgentRegistry(),
		verifyToken: verifyToken,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  65536,
			WriteBufferSize: 65536,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (ts *TunnelServer) Registry() *AgentRegistry {
	return ts.registry
}

func (ts *TunnelServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || ts.verifyToken == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	tenantID := r.Header.Get("X-HNB-Tunnel-Tenant")
	clusterID := r.Header.Get("X-HNB-Tunnel-Cluster")
	if token == "" || strings.ContainsAny(token, " \t\r\n") || tenantID == "" || clusterID == "" ||
		strings.ContainsAny(tenantID+clusterID, "\r\n") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	expiresAt, err := ts.verifyToken(r.Context(), token, tenantID, clusterID)
	if err != nil || !expiresAt.After(time.Now()) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Header.Del("Authorization")
	conn, err := ts.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[tunnel] upgrade failed: %v", err)
		http.Error(w, "upgrade failed", http.StatusBadRequest)
		return
	}

	go ts.handleAgent(conn, clusterID, expiresAt)
}

func (ts *TunnelServer) handleAgent(conn *websocket.Conn, clusterID string, expiresAt time.Time) {
	info := AgentInfo{
		ClusterID:   clusterID,
		ConnectedAt: time.Now().UTC(),
		Status:      "connected",
	}
	agentConn := NewAgentConnection(clusterID, conn, info)
	defer func() {
		ts.registry.Unregister(clusterID, agentConn)
		conn.Close()
	}()

	ts.registry.Register(clusterID, agentConn)
	expiryTimer := time.AfterFunc(time.Until(expiresAt), agentConn.CloseExpired)
	defer expiryTimer.Stop()

	registerResp := &Message{
		Type:      MsgRegister,
		ClusterID: clusterID,
		Payload:   mustMarshalRaw(map[string]string{"status": "registered", "cluster_id": clusterID}),
	}
	agentConn.SendMessage(registerResp)

	for {
		msg, err := agentConn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[tunnel] agent %s disconnected gracefully", clusterID)
			} else {
				log.Printf("[tunnel] agent %s read error: %v", clusterID, err)
			}
			return
		}

		switch msg.Type {
		case MsgHeartbeat:
			var payload HeartbeatPayload
			if msg.Payload != nil {
				json.Unmarshal(msg.Payload, &payload)
			}
			payload.ClusterID = clusterID
			agentConn.UpdateHeartbeat(payload)
			log.Printf("[tunnel] heartbeat from %s (nodes: %d)", clusterID, payload.NodeCount)

			ack := &Message{
				Type:      MsgHeartbeat,
				ClusterID: clusterID,
				Payload:   mustMarshalRaw(map[string]string{"status": "ok"}),
			}
			agentConn.SendMessage(ack)

		case MsgResponse:
			if err := agentConn.DeliverResponse(msg.Payload); err != nil {
				log.Printf("[tunnel] response from %s: %v", clusterID, err)
			}

		case MsgError:
			log.Printf("[tunnel] error from %s: %s", clusterID, string(msg.Payload))

		default:
			log.Printf("[tunnel] unknown message type from %s: %s", clusterID, msg.Type)
		}
	}
}

func (ts *TunnelServer) ProxyRequest(clusterID string, req *RequestPayload) (*ResponsePayload, error) {
	agent, ok := ts.registry.Get(clusterID)
	if !ok {
		return nil, fmt.Errorf("agent %q not connected", clusterID)
	}

	return agent.SendRequest(req)
}

func mustMarshal(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func mustMarshalRaw(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

type AgentClient struct {
	serverURL   string
	tokenSource TokenSource
	tenantID    string
	clusterID   string
	conn        *websocket.Conn
	mu          sync.Mutex
	dialer      websocket.Dialer
}

func NewAgentClient(serverURL string, tokenSource TokenSource, tenantID, clusterID string) *AgentClient {
	return &AgentClient{
		serverURL:   serverURL,
		tokenSource: tokenSource,
		tenantID:    tenantID,
		clusterID:   clusterID,
		dialer: websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
			ReadBufferSize:   65536,
			WriteBufferSize:  65536,
		},
	}
}

func (ac *AgentClient) Connect() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.tokenSource == nil {
		return fmt.Errorf("tunnel token source is required")
	}
	token, err := ac.tokenSource.Token(context.Background())
	if err != nil {
		return fmt.Errorf("load tunnel token: %w", err)
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-HNB-Tunnel-Tenant", ac.tenantID)
	header.Set("X-HNB-Tunnel-Cluster", ac.clusterID)
	conn, _, err := ac.dialer.Dial(ac.serverURL, header)
	if err != nil {
		return fmt.Errorf("dial tunnel server: %w", err)
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("read register response: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		conn.Close()
		return fmt.Errorf("unmarshal register response: %w", err)
	}

	if msg.Type != MsgRegister {
		conn.Close()
		return fmt.Errorf("unexpected message type: %s", msg.Type)
	}

	ac.conn = conn
	log.Printf("[agent] connected to tunnel server as %s", ac.clusterID)
	return nil
}

func (ac *AgentClient) SendHeartbeat(payload HeartbeatPayload) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.conn == nil {
		return fmt.Errorf("not connected")
	}

	msg, err := NewMessage(MsgHeartbeat, ac.clusterID, payload)
	if err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if err := ac.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	return ac.conn.WriteMessage(websocket.TextMessage, data)
}

func (ac *AgentClient) ReadMessage() (*Message, error) {
	ac.mu.Lock()
	conn := ac.conn
	ac.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (ac *AgentClient) SendResponse(requestID string, statusCode int, headers map[string]string, body []byte) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.conn == nil {
		return fmt.Errorf("not connected")
	}

	resp := ResponsePayload{
		RequestID:  requestID,
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
	}

	msg, err := NewMessage(MsgResponse, ac.clusterID, resp)
	if err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if err := ac.conn.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return err
	}
	return ac.conn.WriteMessage(websocket.TextMessage, data)
}

func (ac *AgentClient) Close() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.conn != nil {
		ac.conn.Close()
		ac.conn = nil
	}
}

func (ac *AgentClient) IsConnected() bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.conn != nil
}
