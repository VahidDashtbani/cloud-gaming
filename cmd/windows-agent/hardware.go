package main

import (
	"cloud-gaming/pkg/protocol"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

func CollectHardwareStats(nodeID string, activeSessions, maxSessions int) protocol.HardwareStats {
	v, _ := mem.VirtualMemory()
	c, _ := cpu.Percent(0, false)

	var cpuVal float64
	if len(c) > 0 {
		cpuVal = c[0]
	}

	return protocol.HardwareStats{
		NodeID:         nodeID,
		CPUPercent:     cpuVal,
		RAMUsedMB:      v.Used / 1024 / 1024,
		RAMTotalMB:     v.Total / 1024 / 1024,
		ActiveSessions: activeSessions,
		MaxSessions:    maxSessions,
	}
}
