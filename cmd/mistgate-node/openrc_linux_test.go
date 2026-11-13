//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRCGuardAndArguments(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "node")
	state := filepath.Join(dir, "state")
	os.Mkdir(state, 0700)
	// The generated command must preserve the state directory and guard arguments.
	os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0755)
	script, err := renderOpenRC(bin, state, 256<<20)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "service")
	os.WriteFile(file, []byte(script), 0755)
	run := func() string {
		t.Helper()
		cmd := exec.Command("sh", "-c", `. "$1"; eval "set -- $command_args"; "$command" "$@"`, "test", file)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return string(out)
	}
	if got := run(); got != "run\n--state-dir\n"+state+"\n" {
		t.Fatalf("arguments: %q", got)
	}
	os.WriteFile(filepath.Join(state, "update.pending"), []byte("pending"), 0600)
	os.WriteFile(bin+".prev", []byte("#!/bin/sh\necho restored\n"), 0755)
	run()
	run()
	if got := run(); got != "restored\n" {
		t.Fatalf("rollback: %q", got)
	}
	if _, err := os.Stat(filepath.Join(state, "update.rolledback")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, "MISTGATE_UNIT_GEN=") {
		t.Fatal("must not advertise systemd update support")
	}
}

func TestOpenRCRejectsUnsafePaths(t *testing.T) {
	for _, p := range []string{"/tmp/a b", "/tmp/x$(id)", "/tmp/a\nb", "/tmp/../x", "/"} {
		if _, err := renderOpenRC("/usr/local/bin/node", p, 0); err == nil {
			t.Fatalf("accepted %q", p)
		}
	}
}

func TestOpenRCContainerMemoryBudget(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want uint64
	}{{"159997952\n", 159997952}, {"max\n", 1 << 30}, {"9223372036854771712", 1 << 30}} {
		ram := openRCRAM(1<<30, func(string) ([]byte, error) { return []byte(tc.raw), nil })
		if ram != tc.want {
			t.Fatalf("budget %q: %d, want %d", tc.raw, ram, tc.want)
		}
	}
}
