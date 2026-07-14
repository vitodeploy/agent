package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/vitodeploy/agent/pkg/config"
)

// stubCommand replaces systemctl with a shell script for the duration of a
// test. It returns a counter of how often the command was run.
func stubCommand(t *testing.T, script string) *int {
	t.Helper()
	runs := 0
	original := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		runs++
		return exec.CommandContext(ctx, "sh", "-c", script)
	}
	t.Cleanup(func() { commandContext = original })
	return &runs
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

// shortenTimeouts keeps the timeout tests fast.
func shortenTimeouts(t *testing.T) {
	t.Helper()
	originalTimeout, originalWaitDelay := commandTimeout, waitDelay
	commandTimeout, waitDelay = 200*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { commandTimeout, waitDelay = originalTimeout, originalWaitDelay })
}

// A systemctl that prints a full set of statuses and then hangs is killed on the
// deadline, which surfaces as the same *exec.ExitError an inactive unit causes.
// Its output must be discarded rather than reported as a fresh reading.
func TestGetServiceStatusesDiscardsOutputOnTimeout(t *testing.T) {
	shortenTimeouts(t)
	// exec so that the shell is replaced and nothing outlives the kill.
	stubCommand(t, "echo active; exec sleep 30")

	statuses := GetServiceStatuses([]config.ServiceConfig{{Id: 3, Unit: "nginx"}})

	if statuses != nil {
		t.Fatalf("expected nil statuses on timeout, got %#v", statuses)
	}
}

// Killing systemctl does not close the output pipe while a child it left behind
// still holds it, so without a WaitDelay the collection would block for as long
// as that child lives and stall the metrics loop with it.
func TestGetServiceStatusesReturnsWhenSystemctlLeavesAChildBehind(t *testing.T) {
	shortenTimeouts(t)
	// No exec, so the shell forks and `sleep` inherits the output pipe.
	stubCommand(t, "echo active; sleep 30")

	done := make(chan []ServiceStatus, 1)
	go func() {
		done <- GetServiceStatuses([]config.ServiceConfig{{Id: 3, Unit: "nginx"}})
	}()

	select {
	case statuses := <-done:
		if statuses != nil {
			t.Errorf("expected nil statuses, got %#v", statuses)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("GetServiceStatuses blocked on a lingering child instead of giving up")
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
	runs := stubCommand(t, "echo active")

	if statuses := GetServiceStatuses(nil); statuses != nil {
		t.Fatalf("expected nil statuses for nil config, got %#v", statuses)
	}
	if statuses := GetServiceStatuses([]config.ServiceConfig{}); statuses != nil {
		t.Fatalf("expected nil statuses for empty config, got %#v", statuses)
	}
	if *runs != 0 {
		t.Errorf("expected systemctl not to be run, got %d runs", *runs)
	}
}

func TestGetServiceStatusesOnlyInvalidEntries(t *testing.T) {
	runs := stubCommand(t, "echo active; echo active")

	statuses := GetServiceStatuses([]config.ServiceConfig{
		{Id: 3, Unit: ""},
		{Id: 0, Unit: "nginx"},
	})

	if statuses != nil {
		t.Fatalf("expected nil statuses, got %#v", statuses)
	}
	if *runs != 0 {
		t.Errorf("expected systemctl not to be run, got %d runs", *runs)
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

// The server validates max:100, so this asserts the literal limit rather than
// maxServices, which would make a wrong constant assert itself correct.
func TestSelectEntriesCapsAt100(t *testing.T) {
	services := make([]config.ServiceConfig, 150)
	for i := range services {
		services[i] = config.ServiceConfig{Id: int64(i + 1), Unit: "nginx"}
	}

	entries := selectEntries(services)

	if len(entries) != 100 {
		t.Fatalf("expected 100 entries, got %d", len(entries))
	}
	if entries[0].Id != 1 || entries[99].Id != 100 {
		t.Errorf("expected the first 100 services, got ids %d..%d", entries[0].Id, entries[99].Id)
	}
}

// The server validates max:32 characters, so this asserts the literal limit
// rather than maxStatusChars, which would make a wrong constant assert itself
// correct. Laravel counts characters, not bytes, hence the multi-byte case.
func TestParseStatusesTruncatesLongStatusTo32(t *testing.T) {
	entries := []config.ServiceConfig{{Id: 3, Unit: "nginx"}}

	statuses := parseStatuses(entries, []byte(strings.Repeat("a", 40)+"\n"))

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %#v", statuses)
	}
	if statuses[0].Status != strings.Repeat("a", 32) {
		t.Errorf("expected status truncated to 32 chars, got %q", statuses[0].Status)
	}
}

func TestParseStatusesTruncatesMultiByteStatusTo32Chars(t *testing.T) {
	entries := []config.ServiceConfig{{Id: 3, Unit: "nginx"}}

	statuses := parseStatuses(entries, []byte(strings.Repeat("é", 40)+"\n"))

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %#v", statuses)
	}
	if statuses[0].Status != strings.Repeat("é", 32) {
		t.Errorf("expected status truncated to 32 chars, got %q", statuses[0].Status)
	}
	if count := utf8.RuneCountInString(statuses[0].Status); count != 32 {
		t.Errorf("expected 32 characters, got %d", count)
	}
}

func TestParseStatusesDropsEmptyStatus(t *testing.T) {
	entries := []config.ServiceConfig{{Id: 3, Unit: "nginx"}, {Id: 7, Unit: "php8.4-fpm"}}

	statuses := parseStatuses(entries, []byte("   \nactive\n"))

	if len(statuses) != 1 || statuses[0] != (ServiceStatus{Id: 7, Status: "active"}) {
		t.Fatalf("expected only the php8.4-fpm status, got %#v", statuses)
	}
}

// nil rather than an empty slice, so that the payload omits the key outright.
func TestParseStatusesReturnsNilWhenEveryStatusIsEmpty(t *testing.T) {
	entries := []config.ServiceConfig{{Id: 3, Unit: "nginx"}}

	if statuses := parseStatuses(entries, []byte("   \n")); statuses != nil {
		t.Fatalf("expected nil statuses, got %#v", statuses)
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
