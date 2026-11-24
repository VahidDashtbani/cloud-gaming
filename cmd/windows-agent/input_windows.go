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
	INPUT_MOUSE          = 0
	INPUT_KEYBOARD       = 1
	KEYEVENTF_KEYUP      = 0x0002
	MOUSEEVENTF_MOVE     = 0x0001
	MOUSEEVENTF_LEFTDOWN = 0x0002
	MOUSEEVENTF_LEFTUP   = 0x0004
	MOUSEEVENTF_RIGHTDN  = 0x0008
	MOUSEEVENTF_RIGHTUP  = 0x0010
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

type INPUT_UNION struct {
	Type uint32
	Mi   MOUSEINPUT
}

func InjectInput(pkt protocol.InputPacket) {
	var in INPUT_UNION

	switch pkt.Type {
	case protocol.InputKeyDown, protocol.InputKeyUp:
		in.Type = INPUT_KEYBOARD
		ki := (*KEYBDINPUT)(unsafe.Pointer(&in.Mi))
		ki.WVk = uint16(pkt.Val1)
		if pkt.Type == protocol.InputKeyUp {
			ki.DwFlags = KEYEVENTF_KEYUP
		}

	case protocol.InputMouseMove:
		in.Type = INPUT_MOUSE
		in.Mi.Dx = int32(pkt.Val1)
		in.Mi.Dy = int32(pkt.Val2)
		in.Mi.DwFlags = MOUSEEVENTF_MOVE

	case protocol.InputMouseDown:
		in.Type = INPUT_MOUSE
		if pkt.Val1 == 0 {
			in.Mi.DwFlags = MOUSEEVENTF_LEFTDOWN
		} else {
			in.Mi.DwFlags = MOUSEEVENTF_RIGHTDN
		}

	case protocol.InputMouseUp:
		in.Type = INPUT_MOUSE
		if pkt.Val1 == 0 {
			in.Mi.DwFlags = MOUSEEVENTF_LEFTUP
		} else {
			in.Mi.DwFlags = MOUSEEVENTF_RIGHTUP
		}
	}

	procSendInput.Call(
		uintptr(1),
		uintptr(unsafe.Pointer(&in)),
		uintptr(unsafe.Sizeof(in)),
	)
}
