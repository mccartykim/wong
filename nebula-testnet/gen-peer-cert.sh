#!/usr/bin/env bash
# Generate a peer certificate for a new node joining the testnet.
# Run this on a machine that has the CA key.
#
# Usage: ./gen-peer-cert.sh <name> <overlay-ip> [groups]
# Example: ./gen-peer-cert.sh my-laptop 10.42.0.2/24 "testnet,client"
#          ./gen-peer-cert.sh claude-session 10.42.0.10/24 "testnet,agent"
#
# This outputs the cert and key contents to stdout, suitable for
# copying into environment variables.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
NEBULA_CERT="${NEBULA_CERT_BIN:-/tmp/nebula-cert}"

if [ $# -lt 2 ]; then
  echo "Usage: $0 <name> <overlay-ip/mask> [groups]" >&2
  echo "Example: $0 claude-agent 10.42.0.10/24 testnet,agent" >&2
  exit 1
fi

NAME="$1"
IP="$2"
GROUPS="${3:-testnet}"

CA_CRT="${SCRIPT_DIR}/ca.crt"
CA_KEY="${SCRIPT_DIR}/ca.key"

if [ ! -f "$CA_CRT" ] || [ ! -f "$CA_KEY" ]; then
  echo "ERROR: ca.crt and ca.key must exist in $SCRIPT_DIR" >&2
  exit 1
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

"$NEBULA_CERT" sign \
  -name "$NAME" \
  -ip "$IP" \
  -groups "$GROUPS" \
  -ca-crt "$CA_CRT" \
  -ca-key "$CA_KEY" \
  -out-crt "$TMPDIR/${NAME}.crt" \
  -out-key "$TMPDIR/${NAME}.key"

echo "=== Certificate generated for: $NAME ($IP) ==="
echo ""
echo "# Set these as environment variables / secrets:"
echo ""
echo "--- NEBULA_CA_CRT ---"
cat "$CA_CRT"
echo ""
echo "--- NEBULA_HOST_CRT ---"
cat "$TMPDIR/${NAME}.crt"
echo ""
echo "--- NEBULA_HOST_KEY ---"
cat "$TMPDIR/${NAME}.key"
echo ""
echo "# Cert details:"
"$NEBULA_CERT" print -path "$TMPDIR/${NAME}.crt"
