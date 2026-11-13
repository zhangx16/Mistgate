//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mistgate/mistgate/internal/node/update"
)

const openRCService = "mistgate-node"

func isOpenRC() bool {
	_, err := os.Stat("/sbin/openrc-run")
	if err != nil {
		return false
	}
	_, err = exec.LookPath("rc-service")
	return err == nil
}

// renderOpenRC uses the same path validation as systemd. Each supervisor start
// runs the update crash guard, including a restart after a failed self-update.
func renderOpenRC(bin, stateDir string, ram uint64) (string, error) {
	if _, err := renderUnit(bin, stateDir, ram); err != nil {
		return "", err
	}
	ram = openRCRAM(ram, os.ReadFile)
	memory := max((ram>>20)*60/100, minGoMemMiB)
	script := update.GuardScript
	// GuardScript exits its shell; run it in a subshell before exec-ing the agent.
	wrapper := "(" + script + "); exec " + bin + " run --state-dir " + stateDir
	return fmt.Sprintf(`#!/sbin/openrc-run
name="Mistgate node agent"
description="Mistgate VPN node agent"
supervisor="supervise-daemon"
command="/bin/sh"
command_args="-c '%s' mistgate-guard %s %s"
pidfile="/run/mistgate-node.pid"
respawn_delay=5
respawn_max=3
respawn_period=60
output_logger="logger -t mistgate-node"
error_logger="logger -t mistgate-node"
export GOMEMLIMIT=%dMiB
export MISTGATE_SERVICE_MANAGER=openrc
# Unit generation is deliberately absent: the systemd update/refresh contract
# is not available under OpenRC. Update this custom agent manually.
umask 077

depend() {
 need net
 after firewall
}
start_pre() {
 checkpath --directory --mode 0700 %s
 ulimit -n 65536
}
stop_post() {
 %s cleanup-net
}
`, strings.NewReplacer("$", "\\$", "\"", "\\\"").Replace(wrapper), bin, stateDir, memory, stateDir, bin), nil
}

// Container hosts often expose the host's MemTotal, not their own budget.
func openRCRAM(ram uint64, read func(string) ([]byte, error)) uint64 {
	if ram == 0 {
		ram = 1 << 30
	}
	for _, file := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		raw, err := read(file)
		if err != nil {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
		if err == nil && n > 0 && n < ram {
			ram = n
		}
	}
	return ram
}

func installOpenRC(stateDir, bin string, start bool) error {
	// Dependencies are provisioned over SSH or by the owner, not silently installed
	// by a running node. Both commands are needed by the Linux host controller.
	for _, name := range []string{"rc-update", "rc-service", "supervise-daemon", "nft", "ip"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("OpenRC requires %s (apk add openrc iproute2 nftables)", name)
		}
	}
	script, err := renderOpenRC(bin, stateDir, 0)
	if err != nil {
		return err
	}
	if err := placeBinary(bin); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/sysctl.d", 0755); err != nil {
		return err
	}
	file := filepath.Join("/etc/init.d", openRCService)
	if err := os.WriteFile(file, []byte(script), 0755); err != nil {
		return err
	}
	if out, err := exec.Command("rc-update", "add", openRCService, "default").CombinedOutput(); err != nil {
		return fmt.Errorf("rc-update: %w: %s", err, out)
	}
	if start {
		if out, err := exec.Command("rc-service", openRCService, "restart").CombinedOutput(); err != nil {
			return fmt.Errorf("rc-service: %w: %s", err, out)
		}
	}
	fmt.Printf("installed %s (state %s); logs: logread -e mistgate-node\n", file, stateDir)
	return nil
}
