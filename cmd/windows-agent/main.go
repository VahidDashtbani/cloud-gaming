package main

import (
	"cloud-gaming/pkg/protocol"
	"encoding/json"
	"flag"
	"bufio"
	"io"
	"log"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/h264reader"
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
	authToken   string
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
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
				"stun:stun2.l.google.com:19302",
			}},
		},
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

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("[Session %s] ICE State: %s", req.SessionID, state.String())
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[Session %s] PC State: %s", req.SessionID, state.String())
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("[Session %s] DataChannel opened: label=%s", req.SessionID, dc.Label())
		if dc.Label() != "input" {
			return
		}
		dc.OnOpen(func() {
			log.Printf("[Session %s] Input DataChannel ready", req.SessionID)
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if pkt, ok := protocol.DecodeInput(msg.Data); ok {
				InjectInput(pkt)
			} else {
				log.Printf("[Input Decode Fail] len=%d raw=%x", len(msg.Data), msg.Data)
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
			"-loglevel", "info",
			"-f", "gdigrab", "-framerate", "60", "-draw_mouse", "1", "-i", "desktop",
			"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p",
			"-c:v", "libx264", "-profile:v", "baseline", "-level", "5.1",
			"-preset", "ultrafast", "-tune", "zerolatency",
			"-x264opts", "keyint=60:min-keyint=60:scenecut=0:repeat-headers=1",
			"-bf", "0",
			"-f", "h264", "pipe:1",
		)
	} else {
		cmd = exec.Command("ffmpeg",
			"-re", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=60",
			"-pix_fmt", "yuv420p",
			"-c:v", "libx264", "-profile:v", "baseline", "-level", "3.1",
			"-preset", "ultrafast", "-tune", "zerolatency",
			"-x264opts", "keyint=60:min-keyint=60:scenecut=0:repeat-headers=1",
			"-bf", "0", "-g", "20",
			"-f", "h264", "pipe:1",
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[FFmpeg StdoutPipe Error] %v", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[FFmpeg StderrPipe Error] %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[FFmpeg Start Error] %v", err)
		return
	}
	sess.FFmpegCmd = cmd

	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			log.Printf("[FFmpeg] %s", sc.Text())
		}
	}()

	h264Reader, err := h264reader.NewReader(stdout)
	if err != nil {
		log.Printf("[H264Reader Error] %v", err)
		return
	}

	var sps, pps, pendingPrefix []byte
	frameDur := time.Second / 60
	startCode4 := []byte{0x00, 0x00, 0x00, 0x01}

	for {
		nal, err := h264Reader.NextNAL()
		if err != nil {
			if err != io.EOF {
				log.Printf("[H264 Read Ended] %v", err)
			}
			return
		}
		if nal == nil || len(nal.Data) == 0 {
			continue
		}

		// NAL header byte after start code
		nalType := nal.Data[0] & 0x1F
		annexB := make([]byte, len(startCode4)+len(nal.Data))
		copy(annexB, startCode4)
		copy(annexB[len(startCode4):], nal.Data)

		switch nalType {
		case 7: // SPS
			sps = annexB
			continue
		case 8: // PPS
			pps = annexB
			continue
		case 9, 6: // AUD, SEI - attach to next actual frame sample
			pendingPrefix = append(pendingPrefix, annexB...)
			continue
		case 5: // IDR (Keyframe)
			var payload []byte
			if len(pendingPrefix) > 0 {
				payload = append(payload, pendingPrefix...)
				pendingPrefix = nil
			}
			if len(sps) > 0 {
				payload = append(payload, sps...)
			}
			if len(pps) > 0 {
				payload = append(payload, pps...)
			}
			payload = append(payload, annexB...)
			if werr := sess.VideoTrack.WriteSample(media.Sample{Data: payload, Duration: frameDur}); werr != nil {
				log.Printf("[WriteSample Err] %v", werr)
			}
		default: // Non-IDR frame / slice (e.g. nalType == 1)
			var payload []byte
			if len(pendingPrefix) > 0 {
				payload = append(payload, pendingPrefix...)
				pendingPrefix = nil
			}
			payload = append(payload, annexB...)
			if werr := sess.VideoTrack.WriteSample(media.Sample{Data: payload, Duration: frameDur}); werr != nil {
				log.Printf("[WriteSample Err] %v", werr)
			}
		}
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
	pairToken := flag.String("token", "", "Pairing token generated by Linux Relay (cg1_...)")
	serverURL := flag.String("server", "ws://127.0.0.1:8080/ws/node", "Fallback Linux Relay WS URL")
	authSecret := flag.String("secret", "", "Fallback auth secret if not using -token")
	nodeID := flag.String("node", "win-gpu-node-01", "Unique Node ID")
	maxSess := flag.Int("max", 5, "Max concurrent isolated users")
	flag.Parse()

	targetWS := *serverURL
	tokenToSend := *authSecret

	if *pairToken != "" {
		bundle, err := protocol.DecodeTokenBundle(*pairToken)
		if err != nil {
			log.Fatalf("Invalid pairing token: %v", err)
		}
		targetWS = bundle.RelayWS
		tokenToSend = bundle.Token
	}

	log.Printf("Connecting to Linux Relay: %s", targetWS)
	conn, _, err := websocket.DefaultDialer.Dial(targetWS, nil)
	if err != nil {
		log.Fatalf("Failed to connect to Linux Relay: %v", err)
	}
	defer conn.Close()

	agent := &WindowsAgent{
		nodeID:      *nodeID,
		authToken:   tokenToSend,
		maxSessions: *maxSess,
		ws:          conn,
		sessions:    make(map[string]*GameSession),
	}

	agent.sendJSON(protocol.ControlMessage{
		Type:      protocol.MsgNodeRegister,
		NodeID:    *nodeID,
		AuthToken: tokenToSend,
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
			if err := sess.PeerConn.SetRemoteDescription(offer); err != nil {
				log.Printf("[SetRemoteDesc Err] %s: %v", msg.SessionID, err)
				continue
			}
			answer, err := sess.PeerConn.CreateAnswer(nil)
			if err != nil {
				log.Printf("[CreateAnswer Err] %s: %v", msg.SessionID, err)
				continue
			}
			if err := sess.PeerConn.SetLocalDescription(answer); err != nil {
				log.Printf("[SetLocalDesc Err] %s: %v", msg.SessionID, err)
				continue
			}

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
				if err := sess.PeerConn.AddICECandidate(cand); err != nil {
					log.Printf("[AddICECandidate Err] %s: %v", msg.SessionID, err)
				}
			}
		}
	}
}
