//go:build !windows

package main

import (
	"cloud-gaming/pkg/protocol"
	"log"
)

func InjectInput(pkt protocol.InputPacket) {
	log.Printf("[Test Input] Type=%d Val1=%d Val2=%d", pkt.Type, pkt.Val1, pkt.Val2)
}
