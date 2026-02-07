#!/usr/bin/env bash
# Nebula testnet bootstrap script
# Designed to run in a Claude Code session using environment variables for certs.
#
# Required environment variables (set via secrets):
#   NEBULA_CA_CRT     - PEM contents of the CA certificate
#   NEBULA_HOST_CRT   - PEM contents of this node's certificate
#   NEBULA_HOST_KEY   - PEM contents of this node's private key
#
# Optional environment variables:
#   NEBULA_LIGHTHOUSE_IP   - Overlay IP of lighthouse (default: none if we ARE the lighthouse)
#   NEBULA_LIGHTHOUSE_ADDR - Public IP:port of lighthouse (e.g., "1.2.3.4:4242")
#   NEBULA_AM_LIGHTHOUSE   - "true" if this node is a lighthouse (default: false)
#   NEBULA_LISTEN_PORT     - UDP listen port (default: 4242)
#   NEBULA_TUN_DISABLED    - "true" to disable TUN (headless mode, default: false)
#
set -euo pipefail

NEBULA_DIR=$(mktemp -d /tmp/nebula-session.XXXXXX)
trap 'rm -rf "$NEBULA_DIR"' EXIT

# --- Write certs from env vars to temp files ---
if [ -z "${NEBULA_CA_CRT:-}" ] || [ -z "${NEBULA_HOST_CRT:-}" ] || [ -z "${NEBULA_HOST_KEY:-}" ]; then
  echo "ERROR: NEBULA_CA_CRT, NEBULA_HOST_CRT, and NEBULA_HOST_KEY must be set" >&2
  exit 1
fi

echo "$NEBULA_CA_CRT"   > "$NEBULA_DIR/ca.crt"
echo "$NEBULA_HOST_CRT"  > "$NEBULA_DIR/host.crt"
echo "$NEBULA_HOST_KEY"  > "$NEBULA_DIR/host.key"
chmod 600 "$NEBULA_DIR"/*.key

AM_LIGHTHOUSE="${NEBULA_AM_LIGHTHOUSE:-false}"
LISTEN_PORT="${NEBULA_LISTEN_PORT:-4242}"
TUN_DISABLED="${NEBULA_TUN_DISABLED:-false}"

# --- Build lighthouse static_host_map and lighthouse hosts entries ---
STATIC_HOST_MAP=""
LIGHTHOUSE_HOSTS=""
if [ "$AM_LIGHTHOUSE" != "true" ] && [ -n "${NEBULA_LIGHTHOUSE_IP:-}" ] && [ -n "${NEBULA_LIGHTHOUSE_ADDR:-}" ]; then
  STATIC_HOST_MAP="  \"${NEBULA_LIGHTHOUSE_IP}\": [\"${NEBULA_LIGHTHOUSE_ADDR}\"]"
  LIGHTHOUSE_HOSTS="    - \"${NEBULA_LIGHTHOUSE_IP}\""
fi

# --- Generate config ---
cat > "$NEBULA_DIR/config.yml" <<YAML
pki:
  ca: ${NEBULA_DIR}/ca.crt
  cert: ${NEBULA_DIR}/host.crt
  key: ${NEBULA_DIR}/host.key

static_host_map:
${STATIC_HOST_MAP}

lighthouse:
  am_lighthouse: ${AM_LIGHTHOUSE}
  interval: 60
  hosts:
${LIGHTHOUSE_HOSTS}

listen:
  host: 0.0.0.0
  port: ${LISTEN_PORT}

punchy:
  punch: true
  respond: true

tun:
  disabled: ${TUN_DISABLED}
  dev: nebula1
  drop_local_broadcast: false
  drop_multicast: false
  tx_queue: 500
  mtu: 1300

logging:
  level: info
  format: text

firewall:
  conntrack:
    tcp_timeout: 12m
    udp_timeout: 3m
    default_timeout: 10m
  outbound:
    - port: any
      proto: any
      host: any
  inbound:
    - port: any
      proto: icmp
      host: any
    - port: 22
      proto: tcp
      host: any
    - port: any
      proto: any
      group: testnet
YAML

echo "Nebula config written to $NEBULA_DIR/config.yml"
echo "Starting nebula..."

# --- Download nebula if not present ---
NEBULA_BIN="${NEBULA_BIN:-/tmp/nebula}"
if [ ! -x "$NEBULA_BIN" ]; then
  echo "Downloading nebula..."
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)  NEBULA_ARCH="linux-amd64" ;;
    aarch64) NEBULA_ARCH="linux-arm64" ;;
    *)       echo "Unsupported arch: $ARCH" >&2; exit 1 ;;
  esac
  curl -sL "https://github.com/slackhq/nebula/releases/latest/download/nebula-${NEBULA_ARCH}.tar.gz" | tar xz -C /tmp nebula nebula-cert
  chmod +x /tmp/nebula /tmp/nebula-cert
fi

exec "$NEBULA_BIN" -config "$NEBULA_DIR/config.yml"
