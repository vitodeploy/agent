package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vitodeploy/agent/pkg/services"
)

func samplePayload() Payload {
	return Payload{
		Load:        1.2,
		DiskTotal:   "100M",
		DiskFree:    "40M",
		DiskUsed:    "60M",
		MemoryTotal: "2000M",
		MemoryFree:  "500M",
		MemoryUsed:  "1500M",

		CPUCores:          4,
		CPUPhysicalCores:  2,
		CPUUsagePercent:   12.5,
		CPUPerCoreUsage:   []float64{1.5, 2.5},
		CPUStealPercent:   0.1,
		MemoryUsedPercent: 75,
		SwapTotal:         "1000M",
		SwapFree:          "900M",
		SwapUsed:          "100M",
		SwapUsedPercent:   10,
		OOMKillCount:      3,
		UptimeSeconds:     99999.5,
		RebootRequired:    true,
	}
}

// The marshalled payload of an agent without service statuses must stay
// byte-for-byte identical to the one previous agent versions sent. The expected
// value was captured from the agent before service reporting was added.
func TestMarshalPayloadWithoutServices(t *testing.T) {
	expected := `{"load":1.2,"disk_total":"100M","disk_free":"40M","disk_used":"60M","memory_total":"2000M","memory_free":"500M","memory_used":"1500M","cpu_cores":4,"cpu_physical_cores":2,"cpu_usage_percent":12.5,"cpu_per_core_usage_percent":[1.5,2.5],"cpu_steal_percent":0.1,"memory_used_percent":75,"swap_total":"1000M","swap_free":"900M","swap_used":"100M","swap_used_percent":10,"oom_kill_count":3,"uptime_seconds":99999.5,"reboot_required":true}`

	payload, err := json.Marshal(samplePayload())
	if err != nil {
		t.Fatalf("marshalling failed: %v", err)
	}

	if string(payload) != expected {
		t.Errorf("payload changed:\nexpected %s\ngot      %s", expected, payload)
	}
}

func TestMarshalZeroPayloadWithoutServices(t *testing.T) {
	expected := `{"load":0,"disk_total":"","disk_free":"","disk_used":"","memory_total":"","memory_free":"","memory_used":"","cpu_cores":0,"cpu_physical_cores":0,"cpu_usage_percent":0,"cpu_per_core_usage_percent":null,"cpu_steal_percent":0,"memory_used_percent":0,"swap_total":"","swap_free":"","swap_used":"","swap_used_percent":0,"oom_kill_count":0,"uptime_seconds":0,"reboot_required":false}`

	payload, err := json.Marshal(Payload{})
	if err != nil {
		t.Fatalf("marshalling failed: %v", err)
	}

	if string(payload) != expected {
		t.Errorf("payload changed:\nexpected %s\ngot      %s", expected, payload)
	}
}

// A nil or empty slice must omit the key entirely rather than send null or [],
// which the server reads as an agent that does not report services.
func TestMarshalPayloadOmitsServicesKey(t *testing.T) {
	for name, statuses := range map[string][]services.ServiceStatus{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			payload := samplePayload()
			payload.Services = statuses

			marshalled, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshalling failed: %v", err)
			}

			if strings.Contains(string(marshalled), "services") {
				t.Errorf("expected no services key, got %s", marshalled)
			}
		})
	}
}

func TestMarshalPayloadWithServices(t *testing.T) {
	payload := samplePayload()
	payload.Services = []services.ServiceStatus{
		{Id: 3, Status: "active"},
		{Id: 7, Status: "failed"},
	}

	marshalled, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshalling failed: %v", err)
	}

	expected := `,"services":[{"id":3,"status":"active"},{"id":7,"status":"failed"}]}`
	if !strings.HasSuffix(string(marshalled), expected) {
		t.Errorf("expected payload to end with %s, got %s", expected, marshalled)
	}
}
