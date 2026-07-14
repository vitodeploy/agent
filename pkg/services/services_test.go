package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vitodeploy/agent/pkg/config"
)

// stubCommand replaces systemctl with a shell script for the duration of a test.
func stubCommand(t *testing.T, script string) {
	t.Helper()
	original := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", script)
	}
	t.Cleanup(func() { commandContext = original })
}

func TestGetServiceStatuses(t *testing.T) {
	stubCommand(t, "echo active; echo failed")

	statuses := GetServiceStatuses([]config.ServiceConfig{
		{Id: 3, Unit: "nginx"},
		{Id: 7, Unit: "php8.4-fpm"},
	})

	expected := []ServiceStatus{{Id: 3, Status: "active"}, {Id: 7, Status: "failed"}}
	if len(statuses) != len(expected) {
		t.Fatalf("expected %d statuses, got %#v", len(expected), statuses)
	}
	for i, status := range expected {
		if statuses[i] != status {
			t.Errorf("status %d: expected %#v, got %#v", i, status, statuses[i])
		}
	}
}

// `systemctl is-active` exits 3 when a unit is not active. The exit code carries
// no information the output does not, so it must be ignored.
func TestGetServiceStatusesIgnoresExitCode(t *testing.T) {
	stubCommand(t, "echo inactive; exit 3")

	statuses := GetServiceStatuses([]config.ServiceConfig{{Id: 3, Unit: "nginx"}})

	if len(statuses) != 1 || statuses[0] != (ServiceStatus{Id: 3, Status: "inactive"}) {
		t.Fatalf("expected inactive status, got %#v", statuses)
	}
}

func TestGetServiceStatusesMissingBinary(t *testing.T) {
	original := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "vito-agent-no-such-binary")
	}
	t.Cleanup(func() { commandContext = original })

	if statuses := GetServiceStatuses([]config.ServiceConfig{{Id: 3, Unit: "nginx"}}); statuses != nil {
		t.Fatalf("expected nil statuses, got %#v", statuses)
	}
}

func TestGetServiceStatusesCountMismatch(t *testing.T) {
	stubCommand(t, "echo active")

	statuses := GetServiceStatuses([]config.ServiceConfig{
		{Id: 3, Unit: "nginx"},
		{Id: 7, Unit: "php8.4-fpm"},
	})

	if statuses != nil {
		t.Fatalf("expected nil statuses, got %#v", statuses)
	}
}

func TestGetServiceStatusesNoOutput(t *testing.T) {
	stubCommand(t, "true")

	if statuses := GetServiceStatuses([]config.ServiceConfig{{Id: 3, Unit: "nginx"}}); statuses != nil {
		t.Fatalf("expected nil statuses, got %#v", statuses)
	}
}

func TestGetServiceStatusesWithoutConfiguredServices(t *testing.T) {
	stubCommand(t, "echo active")

	if statuses := GetServiceStatuses(nil); statuses != nil {
		t.Fatalf("expected nil statuses for nil config, got %#v", statuses)
	}
	if statuses := GetServiceStatuses([]config.ServiceConfig{}); statuses != nil {
		t.Fatalf("expected nil statuses for empty config, got %#v", statuses)
	}
}

// systemctl is never run when every configured entry is unusable.
func TestGetServiceStatusesOnlyInvalidEntries(t *testing.T) {
	stubCommand(t, "echo active; echo active")

	statuses := GetServiceStatuses([]config.ServiceConfig{
		{Id: 3, Unit: ""},
		{Id: 0, Unit: "nginx"},
	})

	if statuses != nil {
		t.Fatalf("expected nil statuses, got %#v", statuses)
	}
}

// The other tests stub out the command entirely, so this one runs the real
// exec path against a systemctl shim to pin down how it is invoked. Units are
// passed as separate arguments and never reach a shell, so a unit name
// containing shell metacharacters is inert.
func TestGetServiceStatusesInvokesSystemctl(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	shim := "#!/bin/sh\nfor arg in \"$@\"; do echo \"$arg\" >> " + argsFile + "; done\necho active\necho active\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(shim), 0755); err != nil {
		t.Fatalf("writing shim failed: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	statuses := GetServiceStatuses([]config.ServiceConfig{
		{Id: 3, Unit: "nginx"},
		{Id: 7, Unit: "redis; rm -rf /"},
	})

	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %#v", statuses)
	}
	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("reading recorded args failed: %v", err)
	}
	expected := "is-active\nnginx\nredis; rm -rf /\n"
	if string(recorded) != expected {
		t.Errorf("expected args %q, got %q", expected, recorded)
	}
}

func TestSelectEntries(t *testing.T) {
	entries := selectEntries([]config.ServiceConfig{
		{Id: 3, Unit: "nginx"},
		{Id: 0, Unit: "redis"}, // no id
		{Id: 7, Unit: ""},      // no unit
		{Id: 9, Unit: "   "},   // blank unit
		{Id: 11, Unit: "mysql"},
	})

	expected := []config.ServiceConfig{{Id: 3, Unit: "nginx"}, {Id: 11, Unit: "mysql"}}
	if len(entries) != len(expected) {
		t.Fatalf("expected %d entries, got %#v", len(expected), entries)
	}
	for i, entry := range expected {
		if entries[i] != entry {
			t.Errorf("entry %d: expected %#v, got %#v", i, entry, entries[i])
		}
	}
}

func TestSelectEntriesCapsAtMaxServices(t *testing.T) {
	services := make([]config.ServiceConfig, 150)
	for i := range services {
		services[i] = config.ServiceConfig{Id: int64(i + 1), Unit: "nginx"}
	}

	entries := selectEntries(services)

	if len(entries) != maxServices {
		t.Fatalf("expected %d entries, got %d", maxServices, len(entries))
	}
	if entries[0].Id != 1 || entries[maxServices-1].Id != maxServices {
		t.Errorf("expected the first %d services, got ids %d..%d", maxServices, entries[0].Id, entries[maxServices-1].Id)
	}
}

func TestParseStatusesTruncatesLongStatus(t *testing.T) {
	long := strings.Repeat("a", 40)
	entries := []config.ServiceConfig{{Id: 3, Unit: "nginx"}}

	statuses := parseStatuses(entries, []byte(long+"\n"))

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %#v", statuses)
	}
	if statuses[0].Status != strings.Repeat("a", maxStatusChars) {
		t.Errorf("expected status truncated to %d chars, got %q", maxStatusChars, statuses[0].Status)
	}
}

func TestParseStatusesDropsEmptyStatus(t *testing.T) {
	entries := []config.ServiceConfig{{Id: 3, Unit: "nginx"}, {Id: 7, Unit: "php8.4-fpm"}}

	statuses := parseStatuses(entries, []byte("   \nactive\n"))

	if len(statuses) != 1 || statuses[0] != (ServiceStatus{Id: 7, Status: "active"}) {
		t.Fatalf("expected only the php8.4-fpm status, got %#v", statuses)
	}
}

// Unknown statuses are round-tripped rather than normalized; the server ignores
// what it does not understand.
func TestParseStatusesKeepsRawStatus(t *testing.T) {
	entries := []config.ServiceConfig{{Id: 3, Unit: "nginx"}}

	statuses := parseStatuses(entries, []byte("activating\n"))

	if len(statuses) != 1 || statuses[0].Status != "activating" {
		t.Fatalf("expected raw activating status, got %#v", statuses)
	}
}

func TestParseStatusesWithoutTrailingNewline(t *testing.T) {
	entries := []config.ServiceConfig{{Id: 3, Unit: "nginx"}}

	statuses := parseStatuses(entries, []byte("active"))

	if len(statuses) != 1 || statuses[0].Status != "active" {
		t.Fatalf("expected active status, got %#v", statuses)
	}
}
