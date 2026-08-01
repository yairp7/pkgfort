#!/bin/sh
set -e

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; exit 1; }

check_activated() {
    ESC=$(printf '\033')
    echo "$1" | sed "s/${ESC}\[[0-9;]*m//g" | grep -q "pkgfort activated" \
        || fail "$2 — pkgfort did not activate"
}

npm_block() {
    label="$1"
    dir="$2"
    grep_version="$3"   # version string to grep for in block log, e.g. "4.17.21" or "" for any
    output=$(cd "$dir" && npm install 2>&1) && rc=0 || rc=$?
    echo "$output"
    [ "$rc" -ne 0 ] || fail "npm block $label — expected non-zero exit"
    check_activated "$output" "npm block $label"
    if [ -n "$grep_version" ]; then
        echo "$output" | grep -q "block lodash@${grep_version}" || fail "npm block $label — package was not blocked"
    else
        echo "$output" | grep -q "block lodash@" || fail "npm block $label — package was not blocked"
    fi
    echo "$output" | grep -q "E403" || fail "npm block $label — npm did not receive 403"
    pass "npm block $label"
}

npm_pass() {
    label="$1"
    dir="$2"
    (cd "$dir" && npm install 2>&1) || fail "npm pass $label — expected exit 0"
    pass "npm pass $label"
}

# --- npm ---

cp ./configs/config-block.json "$HOME/.pkgfort/config.json"
npm_block "fixed (4.17.21)"  npm-project        "4.17.21"
npm_block "caret (^4.0.0)"   npm-project-caret  ""
npm_block "tilde (~4.17.0)"  npm-project-tilde  ""
npm_block "latest (*)"       npm-project-latest ""

cp ./configs/config-pass.json "$HOME/.pkgfort/config.json"
npm_pass "fixed (4.17.21)"  npm-project
npm_pass "caret (^4.0.0)"   npm-project-caret
npm_pass "tilde (~4.17.0)"  npm-project-tilde
npm_pass "latest (*)"       npm-project-latest

go_block() {
    label="$1"
    dir="$2"
    pkg="$3"    # full module@version string to grep for in block log
    output=$(cd "$dir" && go mod download 2>&1) && rc=0 || rc=$?
    echo "$output"
    [ "$rc" -ne 0 ] || fail "go block $label — expected non-zero exit"
    check_activated "$output" "go block $label"
    echo "$output" | grep -q "block ${pkg}" || fail "go block $label — package was not blocked"
    echo "$output" | grep -q "403" || fail "go block $label — go did not receive 403"
    pass "go block $label"
}

go_pass() {
    label="$1"
    dir="$2"
    (cd "$dir" && go mod download 2>&1) || fail "go pass $label — expected exit 0"
    pass "go pass $label"
}

# --- go ---

cp ./configs/config-block.json "$HOME/.pkgfort/config.json"
go_block "exact (v1.6.0)" go-project "github.com/google/uuid@v1.6.0"

cp ./configs/config-pass.json "$HOME/.pkgfort/config.json"
go_pass "exact (v1.6.0)" go-project
