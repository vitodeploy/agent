package cpu

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type CPUInfo struct {
	Load                float64
	LogicalCores        int
	PhysicalCores       int
	UsagePercent        float64
	PerCoreUsagePercent []float64
	StealPercent        float64
}

type cpuSample struct {
	total   uint64
	idleAll uint64
	steal   uint64
}

func GetCPUInfo() CPUInfo {
	info := CPUInfo{
		PerCoreUsagePercent: make([]float64, 0),
	}

	// Read /proc/loadavg first so the legacy `load` field samples at the
	// same instant as it did before this change.
	load, err := loadAverage()
	if err != nil {
		fmt.Println("Error:", err)
	}
	info.Load = load

	info.LogicalCores = runtime.NumCPU()
	info.PhysicalCores = physicalCoreCount(info.LogicalCores)

	overall1, perCore1, err := readProcStat()
	if err != nil {
		fmt.Println("Error reading /proc/stat:", err)
		return info
	}
	time.Sleep(250 * time.Millisecond)
	overall2, perCore2, err := readProcStat()
	if err != nil {
		fmt.Println("Error reading /proc/stat:", err)
		return info
	}

	info.UsagePercent, info.StealPercent = sampleDelta(overall1, overall2)

	// Match per-core samples by parsed cpuN index (not slice position) so
	// CPU hotplug between the two reads can't silently misalign columns.
	ids := make([]int, 0, len(perCore1))
	for id := range perCore1 {
		if _, ok := perCore2[id]; ok {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	perCore := make([]float64, 0, len(ids))
	for _, id := range ids {
		usage, _ := sampleDelta(perCore1[id], perCore2[id])
		perCore = append(perCore, usage)
	}
	info.PerCoreUsagePercent = perCore

	return info
}

func loadAverage() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}

	loadavg := strings.Fields(string(data))
	if len(loadavg) == 0 {
		return 0, fmt.Errorf("empty /proc/loadavg")
	}

	return strconv.ParseFloat(loadavg[0], 64)
}

func readProcStat() (cpuSample, map[int]cpuSample, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuSample{}, nil, err
	}

	var overall cpuSample
	perCore := make(map[int]cpuSample)

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		label := fields[0]
		sample, ok := parseCPULine(fields[1:])
		if !ok {
			continue
		}
		if label == "cpu" {
			overall = sample
			continue
		}
		// Per-core line: must be "cpu<N>" exactly. Reject "cpufreq",
		// "cpu_pressure", and any other future "cpu*" labels.
		if !strings.HasPrefix(label, "cpu") {
			continue
		}
		idx, err := strconv.Atoi(label[3:])
		if err != nil {
			continue
		}
		perCore[idx] = sample
	}

	return overall, perCore, nil
}

func parseCPULine(values []string) (cpuSample, bool) {
	// Fields after the cpu label: user, nice, system, idle, iowait, irq,
	// softirq, steal, guest, guest_nice. Older kernels may omit trailing
	// fields, so accept anything from 4 up.
	if len(values) < 4 {
		return cpuSample{}, false
	}
	nums := make([]uint64, 0, len(values))
	for _, v := range values {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return cpuSample{}, false
		}
		nums = append(nums, n)
	}
	var total uint64
	for _, n := range nums {
		total += n
	}
	idle := nums[3]
	var iowait, steal uint64
	if len(nums) > 4 {
		iowait = nums[4]
	}
	if len(nums) > 7 {
		steal = nums[7]
	}
	return cpuSample{
		total:   total,
		idleAll: idle + iowait,
		steal:   steal,
	}, true
}

func sampleDelta(a, b cpuSample) (usagePct, stealPct float64) {
	if b.total <= a.total {
		return 0, 0
	}
	dTotal := float64(b.total - a.total)
	var dIdle float64
	if b.idleAll > a.idleAll {
		dIdle = float64(b.idleAll - a.idleAll)
	}
	var dSteal float64
	if b.steal > a.steal {
		dSteal = float64(b.steal - a.steal)
	}
	usagePct = clampPct(100 * (dTotal - dIdle) / dTotal)
	stealPct = clampPct(100 * dSteal / dTotal)
	return
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func physicalCoreCount(fallback int) int {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return fallback
	}

	seen := make(map[string]struct{})
	var physID, coreID string
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			if physID != "" && coreID != "" {
				seen[physID+":"+coreID] = struct{}{}
			}
			physID, coreID = "", ""
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "physical id":
			physID = val
		case "core id":
			coreID = val
		}
	}
	if physID != "" && coreID != "" {
		seen[physID+":"+coreID] = struct{}{}
	}

	if len(seen) == 0 {
		return fallback
	}
	return len(seen)
}
