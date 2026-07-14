package services

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/vitodeploy/agent/pkg/config"
)

// The server rejects the whole request when more than maxServices entries are
// sent, or when a status is longer than maxStatusChars.
const maxServices = 100
const maxStatusChars = 32

// These are variables so tests can stub out systemctl and shorten the waits.
var commandContext = exec.CommandContext
var commandTimeout = 5 * time.Second
var waitDelay = time.Second

type ServiceStatus struct {
	Id     int64  `json:"id"`
	Status string `json:"status"`
}

// GetServiceStatuses reports the raw `systemctl is-active` status of each
// configured service. It returns nil when there is nothing to report or when
// the statuses cannot be collected, so that metrics delivery is never blocked.
func GetServiceStatuses(services []config.ServiceConfig) []ServiceStatus {
	entries := selectEntries(services)
	if len(entries) == 0 {
		return nil
	}

	output, err := isActive(units(entries))
	if err != nil {
		fmt.Println("Error running systemctl is-active:", err)
		return nil
	}

	return parseStatuses(entries, output)
}

// selectEntries drops unusable config entries and caps the result at the number
// of services the server accepts.
func selectEntries(services []config.ServiceConfig) []config.ServiceConfig {
	entries := make([]config.ServiceConfig, 0, len(services))
	for _, service := range services {
		if service.Id == 0 || strings.TrimSpace(service.Unit) == "" {
			continue
		}
		if len(entries) == maxServices {
			break
		}
		entries = append(entries, service)
	}
	return entries
}

func units(entries []config.ServiceConfig) []string {
	units := make([]string, 0, len(entries))
	for _, entry := range entries {
		units = append(units, entry.Unit)
	}
	return units
}

// isActive returns the stdout of `systemctl is-active`, which prints one line
// per unit in argument order. A non-zero exit code only means that a unit is
// not active, so it is ignored.
func isActive(units []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	args := append([]string{"is-active"}, units...)
	cmd := commandContext(ctx, "systemctl", args...)
	// Killing systemctl on timeout only closes the pipe once every process
	// holding it has exited, so a lingering child would otherwise block the
	// metrics loop forever. WaitDelay bounds that wait.
	cmd.WaitDelay = waitDelay

	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return nil, err
	}

	return output, nil
}

// parseStatuses pairs each line of the is-active output with the entry it was
// requested for. The whole collection is discarded when the line count does not
// match the unit count, rather than risking a misaligned status.
func parseStatuses(entries []config.ServiceConfig, output []byte) []ServiceStatus {
	trimmed := strings.TrimRight(string(output), "\n")
	if trimmed == "" {
		fmt.Println("Unexpected output from systemctl is-active: no output")
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) != len(entries) {
		fmt.Println("Unexpected output from systemctl is-active: expected", len(entries), "lines, got", len(lines))
		return nil
	}

	statuses := make([]ServiceStatus, 0, len(entries))
	for i, entry := range entries {
		status := truncate(strings.TrimSpace(lines[i]))
		if status == "" {
			continue
		}
		statuses = append(statuses, ServiceStatus{Id: entry.Id, Status: status})
	}
	if len(statuses) == 0 {
		return nil
	}

	return statuses
}

func truncate(status string) string {
	runes := []rune(status)
	if len(runes) <= maxStatusChars {
		return status
	}
	return string(runes[:maxStatusChars])
}
