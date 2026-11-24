package protocol

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
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

type TokenBundle struct {
	RelayWS string `json:"relay_ws"`
	Token   string `json:"token"`
}

func EncodeTokenBundle(b TokenBundle) string {
	raw, _ := json.Marshal(b)
	return "cg1_" + base64.RawURLEncoding.EncodeToString(raw)
}

func DecodeTokenBundle(rawToken string) (TokenBundle, error) {
	trimmed := strings.TrimSpace(rawToken)
	if !strings.HasPrefix(trimmed, "cg1_") {
		return TokenBundle{}, errors.New("فرمت توکن نامعتبر است (باید با cg1_ شروع شود)")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(trimmed, "cg1_"))
	if err != nil {
		return TokenBundle{}, errors.New("خطا در دیکد کردن توکن: " + err.Error())
	}
	var b TokenBundle
	if err := json.Unmarshal(payload, &b); err != nil {
		return TokenBundle{}, errors.New("محتوای توکن نامعتبر است")
	}
	if b.RelayWS == "" || b.Token == "" {
		return TokenBundle{}, errors.New("اطلاعات سرور یا توکن در بسته ناقص است")
	}
	return b, nil
}

type ControlMessage struct {
	Type      string          `json:"type"`
	NodeID    string          `json:"node_id,omitempty"`
	AuthToken string          `json:"auth_token,omitempty"`
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
