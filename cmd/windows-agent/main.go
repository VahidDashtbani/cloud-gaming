package main

import (
	"cloud-gaming/pkg/protocol"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type GameSession struct {
	ID         string
	UserID     string
	PeerConn   *webrtc.PeerConnection
	VideoTrack *webrtc.TrackLocalStaticSample
	FFmpegCmd  *exec.Cmd
}

type WindowsAgent struct {
	nodeID      string
	maxSessions int
	ws          *websocket.Conn
	writeMu     sync.Mutex
	mu          sync.Mutex
	sessions    map[string]*GameSession
}

func (a *WindowsAgent) sendJSON(msg protocol.ControlMessage) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_ = a.ws.WriteJSON(msg)
}

func (a *WindowsAgent) allocateSession(req protocol.AllocateRequest) {
	a.mu.Lock()
	if len(a.sessions) >= a.maxSessions {
		a.mu.Unlock()
		resp, _ := json.Marshal(protocol.AllocateResponse{
			SessionID: req.SessionID,
			Success:   false,
			Error:     "ظرفیت نود تکمیل است (حداکثر ۵ کاربر هم‌زمان)",
		})
		a.sendJSON(protocol.ControlMessage{Type: protocol.MsgAllocateResp, SessionID: req.SessionID, Payload: resp})
		return
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		a.mu.Unlock()
		return
	}

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", req.SessionID,
	)
	if err != nil {
		a.mu.Unlock()
		return
	}
	_, _ = pc.AddTrack(videoTrack)

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "input" {
			return
		}
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if pkt, ok := protocol.DecodeInput(msg.Data); ok {
				InjectInput(pkt)
			}
		})
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			candBytes, _ := json.Marshal(c.ToJSON())
			a.sendJSON(protocol.ControlMessage{
				Type:      protocol.MsgWebRTCIce,
				SessionID: req.SessionID,
				Payload:   candBytes,
			})
		}
	})

	sess := &GameSession{
		ID:         req.SessionID,
		UserID:     req.UserID,
		PeerConn:   pc,
		VideoTrack: videoTrack,
	}
	a.sessions[req.SessionID] = sess
	a.mu.Unlock()

	go a.startStreamPipeline(sess)

	resp, _ := json.Marshal(protocol.AllocateResponse{
		SessionID: req.SessionID,
		Success:   true,
	})
	a.sendJSON(protocol.ControlMessage{Type: protocol.MsgAllocateResp, SessionID: req.SessionID, Payload: resp})
	log.Printf("[Session Allocated] ID=%s Active=%d/%d", req.SessionID, len(a.sessions), a.maxSessions)
}

func (a *WindowsAgent) startStreamPipeline(sess *GameSession) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ffmpeg",
			"-f", "gdigrab", "-framerate", "60", "-i", "desktop",
			"-c:v", "h264_nvenc", "-preset", "p1", "-tune", "ull",
			"-zerolatency", "1", "-bf", "0", "-g", "60",
			"-f", "h264", "pipe:1",
		)
	} else {
		cmd = exec.Command("ffmpeg",
			"-re", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=60",
			"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
			"-bf", "0", "-g", "60", "-f", "h264", "pipe:1",
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("[FFmpeg Error] %v", err)
		return
	}
	sess.FFmpegCmd = cmd

	buf := make([]byte, 65536)
	for {
		n, err := stdout.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("[Stream Read Ended] %v", err)
			}
			return
		}
		_ = sess.VideoTrack.WriteSample(media.Sample{
			Data:     buf[:n],
			Duration: time.Second / 60,
		})
	}
}

func (a *WindowsAgent) releaseSession(sessionID string) {
	a.mu.Lock()
	sess, ok := a.sessions[sessionID]
	if ok {
		delete(a.sessions, sessionID)
	}
	a.mu.Unlock()

	if !ok {
		return
	}
	if sess.FFmpegCmd != nil && sess.FFmpegCmd.Process != nil {
		_ = sess.FFmpegCmd.Process.Kill()
	}
	if sess.PeerConn != nil {
		_ = sess.PeerConn.Close()
	}
	log.Printf("[Session Cleaned Up] ID=%s Active=%d/%d", sessionID, len(a.sessions), a.maxSessions)
}

func main() {
	serverURL := flag.String("server", "ws://127.0.0.1:8080/ws/node", "Linux Relay WS URL")
	nodeID := flag.String("node", "win-gpu-node-01", "Unique Node ID")
	maxSess := flag.Int("max", 5, "Max concurrent isolated users")
	flag.Parse()

	conn, _, err := websocket.DefaultDialer.Dial(*serverURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect to Linux Relay: %v", err)
	}
	defer conn.Close()

	agent := &WindowsAgent{
		nodeID:      *nodeID,
		maxSessions: *maxSess,
		ws:          conn,
		sessions:    make(map[string]*GameSession),
	}

	agent.sendJSON(protocol.ControlMessage{
		Type:   protocol.MsgNodeRegister,
		NodeID: *nodeID,
	})

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		for range ticker.C {
			agent.mu.Lock()
			active := len(agent.sessions)
			agent.mu.Unlock()

			stats := CollectHardwareStats(*nodeID, active, *maxSess)
			data, _ := json.Marshal(stats)
			agent.sendJSON(protocol.ControlMessage{
				Type:    protocol.MsgTelemetry,
				NodeID:  *nodeID,
				Payload: data,
			})
		}
	}()

	for {
		var msg protocol.ControlMessage
		if err := conn.ReadJSON(&msg); err != nil {
			log.Fatalf("Control connection lost: %v", err)
		}

		switch msg.Type {
		case protocol.MsgAllocateReq:
			var req protocol.AllocateRequest
			_ = json.Unmarshal(msg.Payload, &req)
			go agent.allocateSession(req)

		case protocol.MsgReleaseSession:
			go agent.releaseSession(msg.SessionID)

		case protocol.MsgGameTransfer:
			var p protocol.GameTransferPayload
			if err := json.Unmarshal(msg.Payload, &p); err == nil {
				ack := HandleGameTransfer(p)
				ackBytes, _ := json.Marshal(ack)
				agent.sendJSON(protocol.ControlMessage{
					Type:    protocol.MsgGameAck,
					NodeID:  agent.nodeID,
					Payload: ackBytes,
				})
			}

		case protocol.MsgWebRTCOffer:
			agent.mu.Lock()
			sess, ok := agent.sessions[msg.SessionID]
			agent.mu.Unlock()
			if !ok {
				continue
			}

			var offer webrtc.SessionDescription
			_ = json.Unmarshal(msg.Payload, &offer)
			_ = sess.PeerConn.SetRemoteDescription(offer)
			answer, _ := sess.PeerConn.CreateAnswer(nil)
			_ = sess.PeerConn.SetLocalDescription(answer)

			ansBytes, _ := json.Marshal(answer)
			agent.sendJSON(protocol.ControlMessage{
				Type:      protocol.MsgWebRTCAnswer,
				SessionID: msg.SessionID,
				Payload:   ansBytes,
			})

		case protocol.MsgWebRTCIce:
			agent.mu.Lock()
			sess, ok := agent.sessions[msg.SessionID]
			agent.mu.Unlock()
			if ok {
				var cand webrtc.ICECandidateInit
				_ = json.Unmarshal(msg.Payload, &cand)
				_ = sess.PeerConn.AddICECandidate(cand)
			}
		}
	}
}
