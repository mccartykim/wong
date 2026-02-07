# Nebula Testnet Experiment - Findings

## Environment

- **Platform**: Linux x86_64 (gVisor/runsc sandbox)
- **Nebula version**: 1.10.3
- **Overlay network**: 10.42.0.0/24
- **CA**: `wong-testnet` (Curve25519, 1-year validity)

## Network Topology (the real constraint)

All outbound traffic goes through a **JWT-authenticated HTTP CONNECT proxy** at
`21.0.0.39:15004`. There is no direct internet access.

| Protocol | Works? | Notes |
|----------|--------|-------|
| HTTPS (port 443) via CONNECT | **Yes** | TLS 1.3, full bidirectional data flow |
| HTTP (port 80) via CONNECT | **Yes** | Data flows normally |
| CONNECT to arbitrary ports | **Yes** (tunnel opens) | BUT proxy does protocol inspection |
| Raw TCP through CONNECT | **No** | Non-HTTP/TLS traffic gets `400 Bad Request` |
| Raw UDP (any port) | **No** | `sendto()` succeeds but packets are black-holed |
| Raw TCP (bypassing proxy) | **No** | All timeouts - no direct internet |
| DNS | **No** | `/etc/resolv.conf` is empty; proxy resolves hosts |

**Key finding**: The CONNECT proxy opens tunnels to any host:port, but inspects
the traffic flowing through. It only allows HTTP or TLS protocol frames. SSH
banners, raw bytes, and other protocols get rejected with `400 Bad Request`.

This means **Nebula (UDP-based) is completely dead for outbound connectivity**.
Even if TUN worked, the UDP packets would never reach the internet.

## Nebula Results (local)

### TUN device: blocked by gVisor

`/dev/net/tun` exists but the ioctl calls needed to configure it (set MTU, set
tx queue length, bring interface up) are not supported by gVisor.

```
level=error msg="Failed to set tun mtu" error="inappropriate ioctl for device"
level=error msg="Failed to set tun tx queue length" error="inappropriate ioctl for device"
level=fatal msg="failed to bring the tun device up: inappropriate ioctl for device"
```

### Headless mode (`tun.disabled: true`): works locally

Nebula runs stably with TUN disabled. It binds to UDP 4242 and acts as a
lighthouse. But since outbound UDP is black-holed, it can't actually reach
any peers on the internet.

### Bootstrap from env vars: works

The `bootstrap.sh` script reads certs from env vars and starts Nebula.
The pattern is reusable for any tunnel tool.

## Nebula's Userspace Networking

Nebula does have a library/userspace networking mode used by the Defined
Networking mobile apps (iOS/Android) where TUN isn't available or practical.
This is in the `overlay` package. However, even with a userspace network stack,
Nebula still needs outbound UDP, which is black-holed in this sandbox.

## Virtualization & Containerization

| Tool | Available? | Notes |
|------|-----------|-------|
| QEMU | No | Not installed, no `/dev/kvm` |
| Docker | Binary exists | Daemon not running, can't start |
| Podman/LXC | No | Not installed |
| Nix | No | Not installed |
| KVM | No | No `/dev/kvm` |
| User namespaces | No | Not supported |

## What Actually Works for Tunneling

Since only HTTPS through the CONNECT proxy works, viable approaches are:

### 1. **Chisel** (recommended)
- Go binary, tunnels TCP over WebSocket/HTTPS
- You run `chisel server --reverse --port 443` on your machine
- Session runs `chisel client https://your-host R:2222:localhost:22`
- All traffic wraps in TLS through the CONNECT proxy

### 2. **Cloudflare Tunnel** (`cloudflared`)
- Uses HTTPS to Cloudflare edge
- You run `cloudflared` on your machine exposing local services
- Session accesses them via Cloudflare URLs through the proxy

### 3. **SSH over TLS** (stunnel/HAProxy wrapping)
- Wrap SSH in TLS on your server (port 443)
- Session connects via CONNECT proxy to port 443
- TLS unwraps to SSH on your end

### 4. **Bore / Ngrok / Tailscale Funnel**
- HTTP-based tunnel services
- Work through the CONNECT proxy naturally

### Certbot idea for agent cert issuance

For your idea of a certbot at `certbot.kimb.dev`:
- Agent presents a bearer token or SSH public key over HTTPS
- Server issues a short-lived cert (e.g., chisel client cert, SSH cert)
- Since HTTPS works fine through the proxy, this flow is viable
- Short-lived certs (24h) mean revocation is mostly moot - just don't renew
- For overlay IP reuse: new cert with same IP replaces the old holder

## Certificate Revocation (Nebula)

Nebula has `pki.blocklist` - list certificate fingerprints to block. However:
- Blocklist is **not distributed via lighthouses** - push to all nodes manually
- `pki.disconnect_invalid: true` disconnects hosts with expired/blocked certs
- No CRL or OCSP equivalent
- For rotation: issue new certs, add old fingerprints to blocklist, distribute

For short-lived agent sessions, issue certs with `-duration 24h` so they
auto-expire. This is the practical approach for ephemeral environments.

## Files

- `ca.crt` / `ca.key` - testnet CA (DO NOT commit ca.key in production)
- `lighthouse.crt` / `lighthouse.key` - lighthouse node cert
- `config.yml` - Nebula config (tun disabled for sandbox compat)
- `bootstrap.sh` - env-var-based bootstrap for CI/agent sessions
- `gen-peer-cert.sh` - generate peer certs for new nodes
