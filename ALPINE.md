# Alpine node support (local v0.1.86-alpine.1 build)

The existing Debian panel can provision **Alpine Linux 3.20+**, amd64 or
arm64, using **OpenRC**. This change adapts nodes; the panel installer
still requires systemd.

The SSH preflight checks OpenRC and uses BusyBox nc when Bash is absent.
After the owner approves the pinned SSH host, provisioning installs
`ca-certificates`, `iproute2` and `nftables` with apk. It does not enable or
replace the host firewall. Provider firewall rules remain the owner's task.
The normal `mistgate-node install` command detects OpenRC, writes
`/etc/init.d/mistgate-node`, enables the default runlevel and starts the
supervised process. Manual installs need `apk add openrc ca-certificates
iproute2 nftables` first and normal enrollment before `install`.

Service commands:

```sh
rc-service mistgate-node status
rc-service mistgate-node restart
rc-service mistgate-node stop
logread -e mistgate-node
```

The OpenRC service keeps the Go memory budget, restrictive file creation
mask and crash rollback guard. It does not provide systemd's namespace,
capability and cgroup sandbox. Network features still require a capable
Linux kernel, nftables and, for userspace AWG/WARP, `/dev/net/tun`.
The journald baseline is omitted where systemctl is absent.

The systemd-only agent self-update/unit-refresh contract, apt OS/security
updates, resolver repair via systemd-resolved, and automatic AWG kernel
module installation are not enabled on Alpine. Update this agent manually;
use the userspace AWG backend. VLESS and Hysteria2 do not require the AWG
kernel module.

## Optional local signed Alpine bundle

An existing panel can keep its official release/update key and supply a
separate locally signed agent bundle **only for Alpine SSH installations**:

```ini
[Service]
Environment=MISTGATE_ALPINE_BUNDLE_DIR=/var/lib/mistgate/alpine-dist
Environment=MISTGATE_ALPINE_BUNDLE_KEY=<base64-ed25519-public-key>
```

The directory contains `manifest.json`, its detached Ed25519 `manifest.sig`,
and `mistgate-node-linux-amd64` / `mistgate-node-linux-arm64`. The manifest
uses the standard Mistgate node-release schema. Signature, expiry, platform,
file size and SHA-256 are checked before any transfer. The remote transfer
checks SHA-256 again. Keep the directory root-owned and renew the signed
manifest before its expiry. The key is explicitly configured, never learned
from the bundle. Debian/Ubuntu provisioning and the official updater are
unchanged. Without an override, the configured normal node bundle must
itself contain an Alpine-capable agent.

## Validation

Unit tests cover unsupported Alpine versions, missing OpenRC, unsafe service
paths, argument quoting, crash rollback, and tampered/untrusted/expired
bundles. An opt-in test runs the actual preflight in an isolated Alpine
container without Bash:

```sh
MISTGATE_ALPINE_TEST_CONTAINER=<running-alpine-container> \
  go test ./internal/panel/provision -run TestAlpine -v
```

The test container needs OpenRC and a writable root filesystem; it must not
contain a real enrolled identity. Service smoke tests must use an isolated
container or VM, never a production node.

## Verified environments

- Alpine 3.22 amd64 in an isolated container: BusyBox preflight without Bash,
  service generation and OpenRC start/restart/stop.
- Alpine 3.21.10 arm64 in a constrained container: static agent enrollment,
  live panel heartbeat, OpenRC restart/reconnect, default runlevel,
  nftables ownership and cgroup-aware memory budget.
- The arm64 host passed the existing loopback-only VLESS integration tests
  for TLS WebSocket, XHTTP stream-up and packet-up with traffic counters,
  plus WebSocket ALPN negotiation and port release. These tests do not
  validate an external Cloudflare/CDN path.
