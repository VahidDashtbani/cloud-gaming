package main

import (
	"cloud-gaming/pkg/protocol"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

//go:embed index.html
var indexHTML []byte

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WorkerNode struct {
	ID       string
	Conn     *websocket.Conn `json:"-"`
	WriteMu  sync.Mutex      `json:"-"`
	Stats    protocol.HardwareStats
	LastSeen time.Time
}

type PlayerSession struct {
	SessionID string
	UserID    string
	NodeID    string
	Browser   *websocket.Conn
	WriteMu   sync.Mutex
}

type Orchestrator struct {
	mu          sync.RWMutex
	pairToken   string
	nodes       map[string]*WorkerNode
	sessions    map[string]*PlayerSession
}

func NewOrchestrator(pairToken string) *Orchestrator {
	return &Orchestrator{
		pairToken: pairToken,
		nodes:     make(map[string]*WorkerNode),
		sessions:  make(map[string]*PlayerSession),
	}
}

func loadOrCreateSecretToken(tokenFile string) string {
	if envTok := strings.TrimSpace(os.Getenv("PAIR_TOKEN")); envTok != "" {
		return envTok
	}
	if data, err := os.ReadFile(tokenFile); err == nil {
		if tok := strings.TrimSpace(string(data)); tok != "" {
			return tok
		}
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	_ = os.MkdirAll(filepath.Dir(tokenFile), 0755)
	_ = os.WriteFile(tokenFile, []byte(tok+"\n"), 0600)
	return tok
}

func (o *Orchestrator) sendToNode(n *WorkerNode, msg protocol.ControlMessage) error {
	n.WriteMu.Lock()
	defer n.WriteMu.Unlock()
	return n.Conn.WriteJSON(msg)
}

func (o *Orchestrator) sendToBrowser(s *PlayerSession, msg protocol.ControlMessage) error {
	s.WriteMu.Lock()
	defer s.WriteMu.Unlock()
	return s.Browser.WriteJSON(msg)
}

func (o *Orchestrator) selectBestNode() *WorkerNode {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var best *WorkerNode
	for _, n := range o.nodes {
		if n.Stats.MaxSessions > 0 && n.Stats.ActiveSessions >= n.Stats.MaxSessions {
			continue
		}
		if best == nil || n.Stats.ActiveSessions < best.Stats.ActiveSessions {
			best = n
		}
	}
	return best
}

func (o *Orchestrator) handleNodeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var currentNodeID string
	var authenticated bool

	for {
		var msg protocol.ControlMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("[Node Disconnected] %s: %v", currentNodeID, err)
			if currentNodeID != "" {
				o.mu.Lock()
				delete(o.nodes, currentNodeID)
				o.mu.Unlock()
			}
			return
		}

		if msg.Type == protocol.MsgNodeRegister {
			if msg.AuthToken != o.pairToken {
				log.Printf("[Unauthorized Node Attempt] NodeID=%s", msg.NodeID)
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "Invalid pairing token"))
				return
			}
			authenticated = true
			currentNodeID = msg.NodeID
			if currentNodeID == "" {
				currentNodeID = fmt.Sprintf("win-node-%d", time.Now().UnixNano())
			}
			o.mu.Lock()
			o.nodes[currentNodeID] = &WorkerNode{
				ID:       currentNodeID,
				Conn:     conn,
				LastSeen: time.Now(),
				Stats: protocol.HardwareStats{
					NodeID:      currentNodeID,
					MaxSessions: 5,
				},
			}
			o.mu.Unlock()
			log.Printf("[Node Registered & Verified] Windows Worker: %s", currentNodeID)
			continue
		}

		if !authenticated {
			_ = conn.Close()
			return
		}

		switch msg.Type {
		case protocol.MsgTelemetry:
			var stats protocol.HardwareStats
			if err := json.Unmarshal(msg.Payload, &stats); err == nil {
				o.mu.Lock()
				if n, ok := o.nodes[currentNodeID]; ok {
					n.Stats = stats
					n.LastSeen = time.Now()
				}
				o.mu.Unlock()
			}

		case protocol.MsgGameAck:
			log.Printf("[Game Transfer ACK] Node=%s Payload=%s", currentNodeID, string(msg.Payload))

		case protocol.MsgAllocateResp, protocol.MsgWebRTCAnswer, protocol.MsgWebRTCIce:
			o.mu.RLock()
			sess, ok := o.sessions[msg.SessionID]
			o.mu.RUnlock()
			if ok {
				_ = o.sendToBrowser(sess, msg)
			}
		}
	}
}

func (o *Orchestrator) handlePlayerWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sessionID := fmt.Sprintf("sess-%d", time.Now().UnixNano())
	node := o.selectBestNode()
	if node == nil {
		_ = conn.WriteJSON(map[string]string{"error": "هیچ نود بازی با ظرفیت خالی در دسترس نیست!"})
		return
	}

	sess := &PlayerSession{
		SessionID: sessionID,
		UserID:    r.URL.Query().Get("user"),
		NodeID:    node.ID,
		Browser:   conn,
	}

	o.mu.Lock()
	o.sessions[sessionID] = sess
	o.mu.Unlock()

	log.Printf("[Session Created] %s -> Node: %s", sessionID, node.ID)

	reqBytes, _ := json.Marshal(protocol.AllocateRequest{
		SessionID: sessionID,
		UserID:    sess.UserID,
		GameExe:   "game.exe",
	})
	_ = o.sendToNode(node, protocol.ControlMessage{
		Type:      protocol.MsgAllocateReq,
		SessionID: sessionID,
		Payload:   reqBytes,
	})

	defer func() {
		o.mu.Lock()
		delete(o.sessions, sessionID)
		o.mu.Unlock()
		_ = o.sendToNode(node, protocol.ControlMessage{
			Type:      protocol.MsgReleaseSession,
			SessionID: sessionID,
		})
		log.Printf("[Session Released] %s", sessionID)
	}()

	for {
		var msg protocol.ControlMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		msg.SessionID = sessionID
		_ = o.sendToNode(node, msg)
	}
}

func main() {
	addr := flag.String("addr", ":8080", "Listen address")
	publicHost := flag.String("public-host", "127.0.0.1:8080", "Public IP or domain for Windows client pairing token")
	tokenFile := flag.String("token-file", "./pairing_secret.key", "Path to store secret key")
	flag.Parse()

	secret := loadOrCreateSecretToken(*tokenFile)
	relayWS := fmt.Sprintf("ws://%s/ws/node", *publicHost)
	clientToken := protocol.EncodeTokenBundle(protocol.TokenBundle{
		RelayWS: relayWS,
		Token:   secret,
	})

	orch := NewOrchestrator(secret)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	http.HandleFunc("/ws/node", orch.handleNodeWS)
	http.HandleFunc("/ws/play", orch.handlePlayerWS)

	http.HandleFunc("/api/nodes", func(w http.ResponseWriter, r *http.Request) {
		orch.mu.RLock()
		defer orch.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(orch.nodes)
	})

	log.Printf("Linux Cloud Gaming Relay running on %s", *addr)
	log.Println("================================================================")
	log.Println("🔑 توکن اتصال کلاینت ویندوز (این را در کلاینت ویندوز وارد کنید):")
	log.Println(clientToken)
	log.Println("================================================================")
	log.Fatal(http.ListenAndServe(*addr, nil))
}
