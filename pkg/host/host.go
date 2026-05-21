package host

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type HostInfo struct {
	UptimeSeconds  float64
	RebootRequired bool
	OOMKillCount   int64
}

func GetHostInfo() HostInfo {
	return HostInfo{
		UptimeSeconds:  uptimeSeconds(),
		RebootRequired: rebootRequired(),
		OOMKillCount:   oomKillCount(),
	}
}

func uptimeSeconds() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		fmt.Println("Error reading /proc/uptime:", err)
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		fmt.Println("Error parsing /proc/uptime:", err)
		return 0
	}
	return v
}

func rebootRequired() bool {
	_, err := os.Stat("/var/run/reboot-required")
	return err == nil
}

func oomKillCount() int64 {
	data, err := os.ReadFile("/proc/vmstat")
	if err != nil {
		fmt.Println("Error reading /proc/vmstat:", err)
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "oom_kill" {
			continue
		}
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return v
	}
	return 0
}
