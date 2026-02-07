# Nebula Testnet Experiment - Findings

## Environment

- **Platform**: Linux x86_64 (gVisor/runsc sandbox)
- **Nebula version**: 1.10.3
- **Overlay network**: 10.42.0.0/24
- **CA**: `wong-testnet` (Curve25519, 1-year validity)

## Results

### TUN device: blocked by sandbox

`/dev/net/tun` exists but the ioctl calls needed to configure it (set MTU, set
tx queue length, bring interface up) are not supported by gVisor. This means
Nebula **cannot create an overlay network interface** in this environment.

```
level=error msg="Failed to set tun mtu" error="inappropriate ioctl for device"
level=error msg="Failed to set tun tx queue length" error="inappropriate ioctl for device"
level=fatal msg="failed to bring the tun device up: inappropriate ioctl for device"
```

### Headless mode (`tun.disabled: true`): works

Nebula runs stably with TUN disabled. It:
- Binds to UDP port 4242
- Loads certs and firewall rules correctly
- Acts as a lighthouse / relay node
- Shuts down cleanly on SIGTERM

This means a Claude Code session **can run as a headless Nebula node** - it just
can't route IP traffic through the overlay directly.

### Bootstrap from env vars: works

The `bootstrap.sh` script successfully reads `NEBULA_CA_CRT`, `NEBULA_HOST_CRT`,
and `NEBULA_HOST_KEY` from environment variables, writes them to a temp dir, and
starts Nebula. This is compatible with secrets-based provisioning.

## What this enables (with your machine as lighthouse)

If you run a lighthouse on your machine (with TUN enabled), and provide this
session with a signed cert + the CA cert as secrets:

1. **This session** runs Nebula headless (`tun.disabled: true`) connecting to
   your lighthouse
2. **Your machine** has TUN enabled and can see the session as a peer on the
   overlay
3. You could then use SSH port forwarding, SOCKS proxy, or similar to tunnel
   select ports between the session and your machine

The catch: without TUN, this node can't originate or receive IP traffic on the
overlay. Nebula's headless mode is designed for lighthouses that only facilitate
peer discovery, not for actual data transfer.

### Alternative: userspace networking

For actual bidirectional tunneling without TUN, options include:
- **Wireguard-go in userspace mode** (wireguard has a userspace TUN impl)
- **Tailscale's `tsnet`** (embeds tailscale as a library, no TUN needed)
- **Plain SSH reverse tunnels** (no overlay needed, just `ssh -R`)
- **Cloudflare Tunnel / ngrok** (HTTP-level tunneling)

## Certificate revocation

Nebula has a `pki.blocklist` config field where you list certificate fingerprints
to block. However:
- The blocklist is **not distributed via lighthouses** - you must push it to all
  nodes manually
- `pki.disconnect_invalid: true` will disconnect hosts with expired/blocked certs
- There is no CRL or OCSP equivalent
- For rotation: issue new certs, add old fingerprints to blocklist, distribute

For short-lived agent sessions, the simplest approach is to issue certs with
short durations (e.g., `-duration 24h`) so they auto-expire. Combined with the
blocklist for emergency revocation, this is workable.

## Files

- `ca.crt` / `ca.key` - testnet CA (DO NOT commit ca.key in production)
- `lighthouse.crt` / `lighthouse.key` - lighthouse node cert
- `config.yml` - Nebula config (tun disabled for sandbox compat)
- `bootstrap.sh` - env-var-based bootstrap for CI/agent sessions
- `gen-peer-cert.sh` - generate peer certs for new nodes
