package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vitodeploy/agent/pkg/config"
	"github.com/vitodeploy/agent/pkg/cpu"
	"github.com/vitodeploy/agent/pkg/disk"
	"github.com/vitodeploy/agent/pkg/host"
	"github.com/vitodeploy/agent/pkg/memory"
)

type Payload struct {
	// --- existing, unchanged ---
	Load        float64 `json:"load"`
	DiskTotal   string  `json:"disk_total"`
	DiskFree    string  `json:"disk_free"`
	DiskUsed    string  `json:"disk_used"`
	MemoryTotal string  `json:"memory_total"`
	MemoryFree  string  `json:"memory_free"`
	MemoryUsed  string  `json:"memory_used"`

	// --- new ---
	CPUCores          int       `json:"cpu_cores"`
	CPUPhysicalCores  int       `json:"cpu_physical_cores"`
	CPUUsagePercent   float64   `json:"cpu_usage_percent"`
	CPUPerCoreUsage   []float64 `json:"cpu_per_core_usage_percent"`
	CPUStealPercent   float64   `json:"cpu_steal_percent"`
	MemoryUsedPercent float64   `json:"memory_used_percent"`
	SwapTotal         string    `json:"swap_total"`
	SwapFree          string    `json:"swap_free"`
	SwapUsed          string    `json:"swap_used"`
	SwapUsedPercent   float64   `json:"swap_used_percent"`
	OOMKillCount      int64     `json:"oom_kill_count"`
	UptimeSeconds     float64   `json:"uptime_seconds"`
	RebootRequired    bool      `json:"reboot_required"`
}

func main() {
	cfg := config.GetConfig()
	for {
		cpuInfo := cpu.GetCPUInfo()
		diskInfo := disk.GetDiskInfo()
		memoryInfo := memory.GetMemoryInfo()
		hostInfo := host.GetHostInfo()
		payload := Payload{
			Load:        cpuInfo.Load,
			DiskTotal:   diskInfo.Total,
			DiskFree:    diskInfo.Free,
			DiskUsed:    diskInfo.Used,
			MemoryTotal: memoryInfo.Total,
			MemoryFree:  memoryInfo.Free,
			MemoryUsed:  memoryInfo.Used,

			CPUCores:          cpuInfo.LogicalCores,
			CPUPhysicalCores:  cpuInfo.PhysicalCores,
			CPUUsagePercent:   cpuInfo.UsagePercent,
			CPUPerCoreUsage:   cpuInfo.PerCoreUsagePercent,
			CPUStealPercent:   cpuInfo.StealPercent,
			MemoryUsedPercent: memoryInfo.UsedPercent,
			SwapTotal:         memoryInfo.SwapTotal,
			SwapFree:          memoryInfo.SwapFree,
			SwapUsed:          memoryInfo.SwapUsed,
			SwapUsedPercent:   memoryInfo.SwapUsedPercent,
			OOMKillCount:      hostInfo.OOMKillCount,
			UptimeSeconds:     hostInfo.UptimeSeconds,
			RebootRequired:    hostInfo.RebootRequired,
		}
		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			panic(err)
		}
		req, err := http.NewRequest("POST", cfg.Url, bytes.NewBuffer(jsonPayload))
		if err != nil {
			panic(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Secret", cfg.Secret)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		fmt.Println("Response Status:", resp.Status)
		resp.Body.Close()
		time.Sleep(time.Minute)
	}
}
