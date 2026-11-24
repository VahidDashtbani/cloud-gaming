package main

import (
	"cloud-gaming/pkg/protocol"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	mu       sync.RWMutex
	nodes    map[string]*WorkerNode
	sessions map[string]*PlayerSession
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		nodes:    make(map[string]*WorkerNode),
		sessions: make(map[string]*PlayerSession),
	}
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

		switch msg.Type {
		case protocol.MsgNodeRegister:
			currentNodeID = msg.NodeID
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
			log.Printf("[Node Registered] Worker: %s", currentNodeID)

		case protocol.MsgTelemetry:
			var stats protocol.HardwareStats
			if err := json.Unmarshal(msg.Payload, &stats); err == nil {
				o.mu.Lock()
				if n, ok := o.nodes[msg.NodeID]; ok {
					n.Stats = stats
					n.LastSeen = time.Now()
				}
				o.mu.Unlock()
			}

		case protocol.MsgGameAck:
			log.Printf("[Game Transfer ACK] Node=%s Payload=%s", msg.NodeID, string(msg.Payload))

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
	orch := NewOrchestrator()

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

	log.Println("Linux Cloud Gaming Relay running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
