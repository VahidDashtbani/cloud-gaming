//go:build windows

package main

import (
	"cloud-gaming/pkg/protocol"
	"syscall"
	"unsafe"
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

const (
	INPUT_MOUSE          uint32 = 0
	INPUT_KEYBOARD       uint32 = 1
	KEYEVENTF_KEYUP      uint32 = 0x0002
	MOUSEEVENTF_MOVE     uint32 = 0x0001
	MOUSEEVENTF_LEFTDOWN uint32 = 0x0002
	MOUSEEVENTF_LEFTUP   uint32 = 0x0004
	MOUSEEVENTF_RIGHTDN  uint32 = 0x0008
	MOUSEEVENTF_RIGHTUP  uint32 = 0x0010
)

type MOUSEINPUT struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// Windows 64-bit INPUT layout: Type (uint32) + 4-byte alignment pad + 32-byte union
type WIN_INPUT struct {
	Type uint32
	_pad uint32
	Data [32]byte
}

func InjectInput(pkt protocol.InputPacket) {
	var in WIN_INPUT

	switch pkt.Type {
	case protocol.InputKeyDown, protocol.InputKeyUp:
		in.Type = INPUT_KEYBOARD
		ki := (*KEYBDINPUT)(unsafe.Pointer(&in.Data[0]))
		ki.WVk = uint16(pkt.Val1)
		if pkt.Type == protocol.InputKeyUp {
			ki.DwFlags = KEYEVENTF_KEYUP
		}

	case protocol.InputMouseMove:
		in.Type = INPUT_MOUSE
		mi := (*MOUSEINPUT)(unsafe.Pointer(&in.Data[0]))
		mi.Dx = int32(pkt.Val1)
		mi.Dy = int32(pkt.Val2)
		mi.DwFlags = MOUSEEVENTF_MOVE

	case protocol.InputMouseDown:
		in.Type = INPUT_MOUSE
		mi := (*MOUSEINPUT)(unsafe.Pointer(&in.Data[0]))
		if pkt.Val1 == 0 {
			mi.DwFlags = MOUSEEVENTF_LEFTDOWN
		} else {
			mi.DwFlags = MOUSEEVENTF_RIGHTDN
		}

	case protocol.InputMouseUp:
		in.Type = INPUT_MOUSE
		mi := (*MOUSEINPUT)(unsafe.Pointer(&in.Data[0]))
		if pkt.Val1 == 0 {
			mi.DwFlags = MOUSEEVENTF_LEFTUP
		} else {
			mi.DwFlags = MOUSEEVENTF_RIGHTUP
		}
	default:
		return
	}

	procSendInput.Call(
		uintptr(1),
		uintptr(unsafe.Pointer(&in)),
		uintptr(unsafe.Sizeof(in)),
	)
}
