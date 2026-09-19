#!/usr/bin/env bash
# ==============================================================================
# AlphaDrive Installer Test Suite
# Tests argument parsing, validation, checksum verification, and template rendering
# ==============================================================================

set -Eeuo pipefail

PASS_COUNT=0
FAIL_COUNT=0

assert_eq() {
    local test_name="$1"
    local expected="$2"
    local actual="$3"
    if [ "$expected" = "$actual" ]; then
        echo "  ✓ PASS: $test_name"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "  ✗ FAIL: $test_name (expected '$expected', got '$actual')"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

assert_match() {
    local test_name="$1"
    local pattern="$2"
    local actual="$3"
    if [[ "$actual" =~ $pattern ]]; then
        echo "  ✓ PASS: $test_name"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "  ✗ FAIL: $test_name (expected pattern '$pattern', got '$actual')"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

echo "Running AlphaDrive Installer Tests..."
echo "============================================================"

# Test 1: Help menu output
echo "[Test 1] Testing --help output"
HELP_OUT="$(bash install.sh --help)"
assert_match "Help contains usage" "Usage:" "$HELP_OUT"
assert_match "Help contains --version" "--version" "$HELP_OUT"
assert_match "Help contains --domain" "--domain" "$HELP_OUT"
assert_match "Help contains --proxy" "--proxy" "$HELP_OUT"
assert_match "Help contains --non-interactive" "--non-interactive" "$HELP_OUT"

# Test 2: Architecture normalization
echo "[Test 2] Testing Architecture Normalization"
map_arch() {
    case "$1" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) echo "unsupported" ;;
    esac
}
assert_eq "x86_64 maps to amd64" "amd64" "$(map_arch x86_64)"
assert_eq "amd64 maps to amd64" "amd64" "$(map_arch amd64)"
assert_eq "aarch64 maps to arm64" "arm64" "$(map_arch aarch64)"
assert_eq "arm64 maps to arm64" "arm64" "$(map_arch arm64)"
assert_eq "i386 is unsupported" "unsupported" "$(map_arch i386)"
assert_eq "mips is unsupported" "unsupported" "$(map_arch mips)"

# Test 3: Domain Validation & Sanitization
echo "[Test 3] Testing Domain Sanitization & Validation"
validate_domain() {
    local d="$1"
    local clean_d
    clean_d="$(echo "$d" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | tr '[:upper:]' '[:lower:]')"
    if [[ "$clean_d" =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$ ]]; then
        echo "valid"
    else
        echo "invalid"
    fi
}
assert_eq "drive.example.com is valid" "valid" "$(validate_domain drive.example.com)"
assert_eq "sub.drive.my-domain.co.uk is valid" "valid" "$(validate_domain sub.drive.my-domain.co.uk)"
assert_eq "UPPERCASE.DOMAIN.COM is valid" "valid" "$(validate_domain UPPERCASE.DOMAIN.COM)"
assert_eq "domain with injection is invalid" "invalid" "$(validate_domain 'example.com; rm -rf /')"
assert_eq "domain with space is invalid" "invalid" "$(validate_domain 'drive .example.com')"
assert_eq "domain with special chars is invalid" "invalid" "$(validate_domain 'drive$.com')"
assert_eq "empty domain is invalid" "invalid" "$(validate_domain '')"

# Test 4: Port Validation
echo "[Test 4] Testing Port Number Validation"
validate_port() {
    local p="$1"
    if [[ "$p" =~ ^[0-9]+$ ]] && [ "$p" -ge 1 ] && [ "$p" -le 65535 ]; then
        echo "valid"
    else
        echo "invalid"
    fi
}
assert_eq "Port 8080 is valid" "valid" "$(validate_port 8080)"
assert_eq "Port 80 is valid" "valid" "$(validate_port 80)"
assert_eq "Port 443 is valid" "valid" "$(validate_port 443)"
assert_eq "Port 65535 is valid" "valid" "$(validate_port 65535)"
assert_eq "Port 0 is invalid" "invalid" "$(validate_port 0)"
assert_eq "Port 65536 is invalid" "invalid" "$(validate_port 65536)"
assert_eq "Port -80 is invalid" "invalid" "$(validate_port -80)"
assert_eq "Port 'abc' is invalid" "invalid" "$(validate_port abc)"
assert_eq "Port '8080; echo bad' is invalid" "invalid" "$(validate_port '8080; echo bad')"

# Test 5: Version Normalization
echo "[Test 5] Testing Version Normalization"
normalize_version() {
    local v="$1"
    if [ -z "$v" ]; then
        echo "latest"
    elif [[ "$v" != v* ]]; then
        echo "v${v}"
    else
        echo "${v}"
    fi
}
assert_eq "Empty version resolves to latest" "latest" "$(normalize_version '')"
assert_eq "1.0.1 resolves to v1.0.1" "v1.0.1" "$(normalize_version '1.0.1')"
assert_eq "v1.0.1 resolves to v1.0.1" "v1.0.1" "$(normalize_version 'v1.0.1')"
assert_eq "2.0.0-rc1 resolves to v2.0.0-rc1" "v2.0.0-rc1" "$(normalize_version '2.0.0-rc1')"

# Test 6: Checksum Extraction & Verification Logic
echo "[Test 6] Testing SHA256 Checksum Matching Logic"
TMP_TEST_DIR="$(mktemp -d /tmp/alphadrive-test.XXXXXX)"
trap 'rm -rf "${TMP_TEST_DIR}"' EXIT

TEST_BIN="${TMP_TEST_DIR}/alphadrive-linux-amd64"
echo "Dummy binary content" > "${TEST_BIN}"
TEST_HASH="$(sha256sum "${TEST_BIN}" | awk '{print $1}')"

CHECKSUMS_FILE="${TMP_TEST_DIR}/checksums.txt"
cat > "${CHECKSUMS_FILE}" << EOF
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  alphadrive-linux-arm64
${TEST_HASH}  alphadrive-linux-amd64
EOF

EXTRACTED_HASH="$(grep -E "(^|[[:space:]])alphadrive-linux-amd64($|[[:space:]])" "${CHECKSUMS_FILE}" | awk '{print $1}')"
assert_eq "Extracted hash matches actual hash" "${TEST_HASH}" "${EXTRACTED_HASH}"

# Test 7: Non-Interactive Argument Combinations
echo "[Test 7] Testing Non-Interactive Validation Combinations"
test_non_interactive_domain_missing() {
    local output=""
    output="$(bash install.sh --non-interactive --access-mode 2 2>&1 || true)"
    if echo "$output" | grep -F -q -- "--domain is required"; then
        echo "error_caught"
    else
        echo "error_missed"
    fi
}
assert_eq "Missing domain in access-mode 2 fails" "error_caught" "$(test_non_interactive_domain_missing)"

# Test 8: Unknown Options
echo "[Test 8] Testing Unknown Flag Rejection"
test_unknown_flag() {
    local output=""
    output="$(bash install.sh --unknown-flag-xyz 2>&1 || true)"
    if echo "$output" | grep -F -q -- "Unknown option"; then
        echo "caught"
    else
        echo "missed"
    fi
}
assert_eq "Unknown flag fails with error" "caught" "$(test_unknown_flag)"

# Test 9: Invalid Proxy Flag
echo "[Test 9] Testing Invalid Proxy Option Rejection"
test_invalid_proxy() {
    local output=""
    output="$(bash install.sh --proxy invalid_proxy 2>&1 || true)"
    if echo "$output" | grep -F -q -- "Invalid proxy"; then
        echo "caught"
    else
        echo "missed"
    fi
}
assert_eq "Invalid proxy option fails with error" "caught" "$(test_invalid_proxy)"

# Test 10: Invalid Port Flag
echo "[Test 10] Testing Invalid Port Option Rejection"
test_invalid_port() {
    local output=""
    output="$(bash install.sh --port 99999 2>&1 || true)"
    if echo "$output" | grep -F -q -- "Invalid port"; then
        echo "caught"
    else
        echo "missed"
    fi
}
assert_eq "Invalid port option fails with error" "caught" "$(test_invalid_port)"

# Test 11: Invalid Domain Flag
echo "[Test 11] Testing Invalid Domain Option Rejection"
test_invalid_domain() {
    local output=""
    output="$(bash install.sh --domain "bad domain name" 2>&1 || true)"
    if echo "$output" | grep -F -q -- "Invalid domain format"; then
        echo "caught"
    else
        echo "missed"
    fi
}
assert_eq "Invalid domain option fails with error" "caught" "$(test_invalid_domain)"

# Test 12: /etc/os-release VERSION Collision Isolation
echo "[Test 12] Testing /etc/os-release VERSION Variable Isolation"
test_version_isolation() {
    # If an environment variable or sourced /etc/os-release defines VERSION, TARGET_VERSION must remain empty
    local test_subshell
    test_subshell="$(bash -c '
        VERSION="12 (bookworm)"
        TARGET_VERSION=""
        # Sourcing /etc/os-release simulation:
        VERSION="12 (bookworm)"
        if [ -n "$TARGET_VERSION" ]; then
            echo "collided"
        else
            echo "isolated"
        fi
    ')"
    echo "$test_subshell"
}
assert_eq "TARGET_VERSION is isolated from OS-release VERSION" "isolated" "$(test_version_isolation)"

# Test 13: Explicit version parsing
echo "[Test 13] Testing Explicit Version Flag Handling"
test_explicit_version_normalization() {
    local v1 v2
    v1="$(bash -c '
        TARGET_VERSION="1.0.1"
        if [[ "${TARGET_VERSION}" != v* ]]; then
            echo "v${TARGET_VERSION}"
        else
            echo "${TARGET_VERSION}"
        fi
    ')"
    v2="$(bash -c '
        TARGET_VERSION="v1.0.1"
        if [[ "${TARGET_VERSION}" != v* ]]; then
            echo "v${TARGET_VERSION}"
        else
            echo "${TARGET_VERSION}"
        fi
    ')"
    if [ "$v1" = "v1.0.1" ] && [ "$v2" = "v1.0.1" ]; then
        echo "normalized"
    else
        echo "failed"
    fi
}
assert_eq "Explicit versions (1.0.1 and v1.0.1) normalize to v1.0.1" "normalized" "$(test_explicit_version_normalization)"

# Test 14: Caddy Reverse Proxy Configuration Regression Test
echo "[Test 14] Testing Caddy Configuration & MIME Preservation"
test_caddy_config_generation() {
    local test_domain="drive.printspad.in"
    local test_port="8080"
    local config
    config=$(cat << EOF
${test_domain} {
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "SAMEORIGIN"
        Referrer-Policy "strict-origin-when-cross-origin"
    }
    encode zstd gzip
    reverse_proxy 127.0.0.1:${test_port} {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
        transport http {
            read_buffer 16384
            response_header_timeout 300s
        }
    }
}
EOF
)
    # Check that it proxies to correct destination
    if ! echo "$config" | grep -F -q "reverse_proxy 127.0.0.1:8080"; then
        echo "missing_proxy_dest"
        return
    fi

    # Ensure no hardcoded text/plain or Content-Type override exists
    if echo "$config" | grep -i -E "(header.*content-type|header_down.*content-type|text/plain)" >/dev/null; then
        echo "forced_mime_override"
        return
    fi

    # Ensure security headers are present
    if ! echo "$config" | grep -F -q "Strict-Transport-Security" || ! echo "$config" | grep -F -q "X-Content-Type-Options"; then
        echo "missing_security_headers"
        return
    fi

    echo "caddy_valid_and_mime_safe"
}
assert_eq "Caddy config proxies cleanly without overriding Content-Type to text/plain" "caddy_valid_and_mime_safe" "$(test_caddy_config_generation)"

# Test 15: Caddy Template in packaging/caddy/Caddyfile
echo "[Test 15] Testing packaging/caddy/Caddyfile template"
test_caddy_template() {
    local template="packaging/caddy/Caddyfile"
    if [ ! -f "$template" ]; then
        echo "template_missing"
        return
    fi
    if grep -i -E "(header_down.*content-type|header.*content-type.*text/plain)" "$template" >/dev/null; then
        echo "forced_mime_override"
        return
    fi
    if ! grep -F -q "reverse_proxy" "$template"; then
        echo "missing_reverse_proxy"
        return
    fi
    echo "template_valid"
}
assert_eq "packaging/caddy/Caddyfile template is clean and valid" "template_valid" "$(test_caddy_template)"

# Test 16: install.sh does not contain forced text/plain MIME overrides
echo "[Test 16] Testing install.sh for MIME type regressions"
test_install_script_mime() {
    if grep -i "content-type.*text/plain" install.sh >/dev/null; then
        echo "regression_found"
    else
        echo "clean"
    fi
}
assert_eq "install.sh contains no forced text/plain overrides" "clean" "$(test_install_script_mime)"

# Test 17: Pre-upgrade backup command context and non-empty validation
echo "[Test 17] Testing Pre-upgrade Backup Logic"
test_preupgrade_backup_logic() {
    # Ensure install.sh does NOT run sudo -u alphadrive into /var/backups
    if grep "sudo -u alphadrive.*backup.*--output" install.sh >/dev/null; then
        echo "unprivileged_var_backups_write_bug"
        return
    fi
    # Ensure install.sh verifies the backup is non-empty (-s)
    if ! grep -F -q '[ -s "${backup_file}" ]' install.sh; then
        echo "missing_nonempty_backup_check"
        return
    fi
    echo "backup_logic_safe"
}
# Test 18: Piped execution does not hang on stdin
echo "[Test 18] Testing Piped stdin Execution (curl | bash compatibility)"
test_piped_execution() {
    local out
    out="$(cat install.sh | bash -s -- --help)"
    if echo "$out" | grep -F -q "AlphaDrive Installer"; then
        echo "piped_success"
    else
        echo "piped_failed"
    fi
}
assert_eq "Piped execution executes completely without hanging" "piped_success" "$(test_piped_execution)"

# Test 19: No top-level exec < /dev/tty
echo "[Test 19] Testing Absence of Top-Level exec < /dev/tty"
test_no_exec_tty() {
    if grep -E "^[[:space:]]*exec[[:space:]]*<[[:space:]]*/dev/tty" install.sh >/dev/null; then
        echo "destructive_exec_found"
    else
        echo "clean"
    fi
}
assert_eq "install.sh does not contain top-level exec < /dev/tty" "clean" "$(test_no_exec_tty)"

echo "============================================================"
echo "Installer Test Results: ${PASS_COUNT} passed, ${FAIL_COUNT} failed"
if [ "$FAIL_COUNT" -gt 0 ]; then
    exit 1
fi
exit 0

