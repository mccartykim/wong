#!/usr/bin/env bash
# Integration test for wong-q2: shell injection in wong-helpers.sh
# Proves that special characters in issue fields don't cause injection.
#
# This test creates a temporary jj repo with wong-db, sources wong-helpers.sh,
# and exercises wong-write, wong-close, and wong-subtask with adversarial inputs.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELPERS="$SCRIPT_DIR/../../wong-helpers.sh"
PASS=0
FAIL=0
TMPDIR=""

cleanup() {
    if [ -n "$TMPDIR" ] && [ -d "$TMPDIR" ]; then
        rm -rf "$TMPDIR"
    fi
}
trap cleanup EXIT

# Create a fresh jj repo with wong-db bookmark for testing
setup_repo() {
    TMPDIR=$(mktemp -d)
    cd "$TMPDIR"

    jj git init 2>/dev/null
    # Create wong-db bookmark off root
    jj new 'root()' -m "wong-db init" 2>/dev/null
    jj bookmark create wong-db -r @ 2>/dev/null || \
        jj bookmark set wong-db -r @ 2>/dev/null
    mkdir -p .wong/issues
    printf '{}' > .wong/issues/.gitkeep.json
    jj squash --into wong-db ".wong/" -u \
        --config 'revset-aliases."immutable_heads()"="none()"' 2>&1 >/dev/null || true

    # Go back to a working change
    jj new 2>/dev/null

    # Source the helpers
    source "$HELPERS"
}

assert_json_field() {
    local json="$1"
    local field="$2"
    local expected="$3"
    local test_name="$4"

    local actual
    actual=$(printf '%s' "$json" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d.get('$field', ''))
") || {
        echo "FAIL: $test_name - could not parse JSON"
        FAIL=$((FAIL + 1))
        return
    }

    if [ "$actual" = "$expected" ]; then
        echo "PASS: $test_name"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $test_name"
        echo "  expected: $expected"
        echo "  actual:   $actual"
        FAIL=$((FAIL + 1))
    fi
}

# Test 1: wong-subtask with single quotes in title
test_subtask_single_quotes() {
    local title="it's a test's test"
    local json
    json=$(WONG_ID="inj-1" WONG_TITLE="$title" WONG_PARENT="parent-1" WONG_DESC="desc" python3 -c "
import json, os
d = {
    'id': os.environ['WONG_ID'],
    'title': os.environ['WONG_TITLE'],
    'status': 'open',
    'parent': os.environ['WONG_PARENT'],
    'description': os.environ['WONG_DESC']
}
print(json.dumps(d, indent=2))
")
    assert_json_field "$json" "title" "$title" "subtask: single quotes in title"
}

# Test 2: wong-subtask with backticks and $() in title
test_subtask_command_injection() {
    local title='$(whoami) `id` ${HOME}'
    local json
    json=$(WONG_ID="inj-2" WONG_TITLE="$title" WONG_PARENT="parent-1" WONG_DESC="desc" python3 -c "
import json, os
d = {
    'id': os.environ['WONG_ID'],
    'title': os.environ['WONG_TITLE'],
    'status': 'open',
    'parent': os.environ['WONG_PARENT'],
    'description': os.environ['WONG_DESC']
}
print(json.dumps(d, indent=2))
")
    assert_json_field "$json" "title" "$title" "subtask: command injection chars in title"
}

# Test 3: wong-subtask with triple quotes in description (would break old '''$desc''')
test_subtask_triple_quotes() {
    local desc="has '''triple''' quotes and \"doubles\""
    local json
    json=$(WONG_ID="inj-3" WONG_TITLE="test" WONG_PARENT="parent-1" WONG_DESC="$desc" python3 -c "
import json, os
d = {
    'id': os.environ['WONG_ID'],
    'title': os.environ['WONG_TITLE'],
    'status': 'open',
    'parent': os.environ['WONG_PARENT'],
    'description': os.environ['WONG_DESC']
}
print(json.dumps(d, indent=2))
")
    assert_json_field "$json" "description" "$desc" "subtask: triple quotes in description"
}

# Test 4: wong-close reason with quotes and special chars
test_close_special_reason() {
    local reason="it's \"done\" with \$(side-effects) and \`backticks\`"
    local input='{"id":"inj-4","status":"open","title":"test"}'
    local updated
    updated=$(echo "$input" | WONG_REASON="$reason" python3 -c "
import sys, json, os
d = json.load(sys.stdin)
d['status'] = 'closed'
d['resolution'] = os.environ['WONG_REASON']
print(json.dumps(d, indent=2))
")
    assert_json_field "$updated" "resolution" "$reason" "close: special chars in reason"
    assert_json_field "$updated" "status" "closed" "close: status is closed"
}

# Test 5: wong-subtask with newlines in description
test_subtask_newlines() {
    local desc=$'line1\nline2\nline3'
    local json
    json=$(WONG_ID="inj-5" WONG_TITLE="test" WONG_PARENT="parent-1" WONG_DESC="$desc" python3 -c "
import json, os
d = {
    'id': os.environ['WONG_ID'],
    'title': os.environ['WONG_TITLE'],
    'status': 'open',
    'parent': os.environ['WONG_PARENT'],
    'description': os.environ['WONG_DESC']
}
print(json.dumps(d, indent=2))
")
    assert_json_field "$json" "description" "$desc" "subtask: newlines in description"
}

# Test 6: End-to-end wong-write/wong-read with special chars (needs jj repo)
test_e2e_write_read_special_chars() {
    setup_repo

    local json='{"id":"e2e-1","title":"it'\''s a \"test\" with $(cmd)","status":"open"}'
    wong-write "e2e-1" "$json" 2>/dev/null

    local read_back
    read_back=$(wong-read "e2e-1" 2>/dev/null) || {
        echo "FAIL: e2e write/read - wong-read failed"
        FAIL=$((FAIL + 1))
        return
    }

    # Verify the JSON round-trips correctly
    local title
    title=$(printf '%s' "$read_back" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d['title'])
") || {
        echo "FAIL: e2e write/read - could not parse read-back JSON"
        FAIL=$((FAIL + 1))
        return
    }

    local expected="it's a \"test\" with \$(cmd)"
    if [ "$title" = "$expected" ]; then
        echo "PASS: e2e write/read with special chars"
        PASS=$((PASS + 1))
    else
        echo "FAIL: e2e write/read with special chars"
        echo "  expected: $expected"
        echo "  actual:   $title"
        FAIL=$((FAIL + 1))
    fi
}

# Test 7: Full wong-subtask + wong-close e2e with adversarial strings
test_e2e_subtask_close_adversarial() {
    # Reuse repo from test 6 (or set up if not done)
    if [ -z "$TMPDIR" ] || [ ! -d "$TMPDIR/.jj" ]; then
        setup_repo
    fi

    # Create a subtask with adversarial title
    wong-subtask "adv-1" "task with 'quotes' and \$(echo pwned)" "parent-x" "desc with '''triple'''" 2>/dev/null

    local read_back
    read_back=$(wong-read "adv-1" 2>/dev/null) || {
        echo "FAIL: e2e subtask adversarial - wong-read failed"
        FAIL=$((FAIL + 1))
        return
    }

    local title
    title=$(printf '%s' "$read_back" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d['title'])
")
    local expected="task with 'quotes' and \$(echo pwned)"
    if [ "$title" = "$expected" ]; then
        echo "PASS: e2e subtask with adversarial title"
        PASS=$((PASS + 1))
    else
        echo "FAIL: e2e subtask with adversarial title"
        echo "  expected: $expected"
        echo "  actual:   $title"
        FAIL=$((FAIL + 1))
    fi

    local desc
    desc=$(printf '%s' "$read_back" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d['description'])
")
    if [ "$desc" = "desc with '''triple'''" ]; then
        echo "PASS: e2e subtask with triple-quoted description"
        PASS=$((PASS + 1))
    else
        echo "FAIL: e2e subtask with triple-quoted description"
        echo "  expected: desc with '''triple'''"
        echo "  actual:   $desc"
        FAIL=$((FAIL + 1))
    fi

    # Now close it with adversarial reason
    wong-close "adv-1" "closed because it's \"done\" and \$(rm -rf /)" 2>/dev/null

    read_back=$(wong-read "adv-1" 2>/dev/null) || {
        echo "FAIL: e2e close adversarial - wong-read failed"
        FAIL=$((FAIL + 1))
        return
    }

    local resolution
    resolution=$(printf '%s' "$read_back" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d['resolution'])
")
    local expected_res="closed because it's \"done\" and \$(rm -rf /)"
    if [ "$resolution" = "$expected_res" ]; then
        echo "PASS: e2e close with adversarial reason"
        PASS=$((PASS + 1))
    else
        echo "FAIL: e2e close with adversarial reason"
        echo "  expected: $expected_res"
        echo "  actual:   $resolution"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== wong-q2 shell injection integration tests ==="
echo ""

# Unit tests (no jj repo needed)
echo "--- Unit tests: Python env-var passing ---"
test_subtask_single_quotes
test_subtask_command_injection
test_subtask_triple_quotes
test_close_special_reason
test_subtask_newlines

echo ""
echo "--- E2E tests: full wong workflow with adversarial inputs ---"
test_e2e_write_read_special_chars
test_e2e_subtask_close_adversarial

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
