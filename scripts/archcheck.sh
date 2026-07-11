#!/usr/bin/env bash
# archcheck.sh proves the architecture lint actually rejects an illegal import,
# rather than trusting that the depguard rules are configured correctly.
#
# It writes a file that violates domain purity (a domain package importing the
# platform database layer), runs `make lint`, and asserts the lint FAILS.
set -euo pipefail

cd "$(dirname "$0")/.."

VIOLATION="internal/modules/example/internal/domain/zz_archcheck_violation.go"
LOG="$(mktemp)"

cleanup() { rm -f "$VIOLATION" "$LOG"; }
trap cleanup EXIT

cat > "$VIOLATION" <<'EOF'
package domain

// Deliberate architecture violation used by scripts/archcheck.sh: a domain
// package must not import the platform database layer. depguard must reject it.
import _ "github.com/williamokano/entitlements/internal/platform/postgres"
EOF

if make lint >"$LOG" 2>&1; then
	echo "archcheck FAILED: lint accepted an illegal domain import (arch rules are not enforcing)"
	echo "--- lint output ---"
	cat "$LOG"
	exit 1
fi

if ! grep -q "internal/platform/postgres" "$LOG"; then
	echo "archcheck FAILED: lint failed, but not because of the illegal import"
	echo "--- lint output ---"
	cat "$LOG"
	exit 1
fi

echo "archcheck OK: lint rejected the illegal domain import"
