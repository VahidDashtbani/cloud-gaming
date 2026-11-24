package protocol

import (
	"encoding/binary"
	"encoding/json"
)

const (
	MsgNodeRegister   = "node_register"
	MsgTelemetry      = "telemetry"
	MsgAllocateReq    = "allocate_session"
	MsgAllocateResp   = "allocate_resp"
	MsgReleaseSession = "release_session"
	MsgWebRTCOffer    = "webrtc_offer"
	MsgWebRTCAnswer   = "webrtc_answer"
	MsgWebRTCIce      = "webrtc_ice"
	MsgGameTransfer   = "game_transfer"
	MsgGameAck        = "game_ack"
)

type ControlMessage struct {
	Type      string          `json:"type"`
	NodeID    string          `json:"node_id,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	UserID    string          `json:"user_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type HardwareStats struct {
	NodeID         string  `json:"node_id"`
	CPUPercent     float64 `json:"cpu_percent"`
	RAMUsedMB      uint64  `json:"ram_used_mb"`
	RAMTotalMB     uint64  `json:"ram_total_mb"`
	ActiveSessions int     `json:"active_sessions"`
	MaxSessions    int     `json:"max_sessions"`
}

type AllocateRequest struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	GameExe   string `json:"game_exe"`
}

type AllocateResponse struct {
	SessionID string `json:"session_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

type GameTransferPayload struct {
	GameName string `json:"game_name"`
	FileName string `json:"file_name"`
	DataB64  string `json:"data_b64"`
	Unzip    bool   `json:"unzip"`
}

type GameAckPayload struct {
	GameName string `json:"game_name"`
	Success  bool   `json:"success"`
	DestPath string `json:"dest_path,omitempty"`
	Error    string `json:"error,omitempty"`
}

const (
	InputKeyDown   byte = 1
	InputKeyUp     byte = 2
	InputMouseMove byte = 3
	InputMouseDown byte = 4
	InputMouseUp   byte = 5
)

type InputPacket struct {
	Type byte
	Val1 int16
	Val2 int16
}

func DecodeInput(buf []byte) (InputPacket, bool) {
	if len(buf) < 5 {
		return InputPacket{}, false
	}
	return InputPacket{
		Type: buf[0],
		Val1: int16(binary.BigEndian.Uint16(buf[1:3])),
		Val2: int16(binary.BigEndian.Uint16(buf[3:5])),
	}, true
}
