#!/usr/bin/env bash

# Exit immediately if a pipeline returns a non-zero status, but handle our custom failures gracefully.
set -euo pipefail

# ANSI color codes for beautiful output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0;0m' # No Color

# Determine base URL from argument or default
URL_BASE="${1:-http://localhost:8000}"
URL_RUN="${URL_BASE}/run"
URL_READY="${URL_BASE}/readyz"

echo -e "${BLUE}==================================================${NC}"
echo -e "${BLUE}    GoBoxD API Integration Test Suite             ${NC}"
echo -e "${BLUE}==================================================${NC}"
echo -e "Target Base URL: ${YELLOW}${URL_BASE}${NC}\n"

# Stats counters
PASSED_COUNT=0
FAILED_COUNT=0

# Helper to run readiness probe
check_readiness() {
    echo -n "Checking server readiness (/readyz)... "
    local response
    local status_code

    # Fetch response and HTTP status code
    response=$(curl -s -w "\nHTTP_CODE:%{http_code}" "${URL_READY}" || true)
    
    if [ -z "$response" ]; then
        echo -e "${RED}FAILED (No response or server unreachable)${NC}"
        return 1
    fi

    # Extract HTTP status code and response body
    status_code=$(echo "$response" | grep "HTTP_CODE:" | cut -d':' -f2)
    body=$(echo "$response" | grep -v "HTTP_CODE:")

    if [ "$status_code" != "200" ]; then
        echo -e "${RED}FAILED (HTTP Code: ${status_code})${NC}"
        echo -e "Response body: ${body}"
        return 1
    fi

    if [[ "$body" != *'"status":"ok"'* ]]; then
        echo -e "${RED}FAILED (Degraded status)${NC}"
        echo -e "Response body: ${body}"
        return 1
    fi

    echo -e "${GREEN}OK${NC}"
    return 0
}

# Helper to run a test case json file and verify expected status
# Arguments:
#   1: Test Name/Description
#   2: JSON File Path (relative to project root/tests dir)
#   3: Expected status value (e.g., "ok", "time_limit_exceeded")
run_test_case() {
    local name="$1"
    local file_path="$2"
    local expected_status="$3"

    echo -n "Running ${name}... "

    if [ ! -f "$file_path" ]; then
        echo -e "${RED}ERROR: Test file not found at ${file_path}${NC}"
        FAILED_COUNT=$((FAILED_COUNT + 1))
        return 1
    fi

    local response
    response=$(curl -s -X POST "${URL_RUN}" \
        -H 'Content-Type: application/json' \
        -d @"${file_path}" || true)

    if [ -z "$response" ]; then
        echo -e "${RED}FAILED (No response from ${URL_RUN})${NC}"
        FAILED_COUNT=$((FAILED_COUNT + 1))
        return 1
    fi

    # Extract status from the response JSON
    local status
    status=$(echo "$response" | grep -o '"status":"[^"]*"' | head -n1 | cut -d':' -f2 | tr -d '"' || true)

    if [ "$status" = "$expected_status" ]; then
        # For "ok" status, also check that individual tests passed (they shouldn't be wrong_output)
        if [ "$expected_status" = "ok" ]; then
            if [[ "$response" == *'"status":"wrong_output"'* ]]; then
                echo -e "${RED}FAILED (Test case returned wrong_output)${NC}"
                echo -e "Response: ${response}"
                FAILED_COUNT=$((FAILED_COUNT + 1))
                return 1
            fi
        fi
        
        echo -e "${GREEN}PASSED${NC}"
        PASSED_COUNT=$((PASSED_COUNT + 1))
        return 0
    else
        echo -e "${RED}FAILED${NC}"
        echo -e "  Expected status: ${YELLOW}${expected_status}${NC}"
        echo -e "  Received status: ${RED}${status:-unknown}${NC}"
        echo -e "  Response: ${response}"
        FAILED_COUNT=$((FAILED_COUNT + 1))
        return 1
    fi
}

# 1. Run readiness check first
if ! check_readiness; then
    echo -e "\n${RED}Readiness check failed. Aborting integration tests!${NC}"
    exit 1
fi
echo ""

# 2. Run all individual language and isolation tests
# Tuple format: "Name" "JSON File Path" "Expected Status"
TESTS=(
    "C Test"                    "tests/test_c.json"                "ok"
    "C++ Test"                  "tests/test_cpp.json"              "ok"
    "Java Test"                 "tests/test_java.json"             "ok"
    "JavaScript Test"           "tests/test_js.json"               "ok"
    "Verilog Test"              "tests/test_verilog.json"          "ok"
    "Ruby Test"                 "tests/test_ruby.json"             "ok"
    "Rust Test"                 "tests/test_rust.json"             "ok"
    "Go Test"                   "tests/test_go.json"               "ok"
    "Python 3 Hello Test"       "tests/test_python.json"           "ok"
    "Python 2 Test"             "tests/test_python2.json"          "ok"
    "Bash Test"                 "tests/test_bash.json"             "ok"
    "Bash Isolation Test"       "tests/test_bash_isolation.json"   "ok"
    "Python FS Isolation Test"   "tests/test_fs_isolation.json"     "ok"
    "Python Network Isolation"  "tests/test_net_isolation.json"    "ok"
    "Python Infinite Loop TLE"  "tests/test_python_loop.json"      "time_limit_exceeded"
    "Python Sleep TLE"          "tests/test_tle.json"              "time_limit_exceeded"
)

# Loop over the tests array
for ((i=0; i<${#TESTS[@]}; i+=3)); do
    run_test_case "${TESTS[i]}" "${TESTS[i+1]}" "${TESTS[i+2]}"
done

# 3. Print Summary
echo -e "\n${BLUE}==================================================${NC}"
echo -e "${BLUE}    Test Run Summary                              ${NC}"
echo -e "${BLUE}==================================================${NC}"
echo -e "  Total Tests Run: $((PASSED_COUNT + FAILED_COUNT))"
echo -e "  Passed:          ${GREEN}${PASSED_COUNT}${NC}"
echo -e "  Failed:          ${RED}${FAILED_COUNT}${NC}"
echo -e "${BLUE}==================================================${NC}"

if [ "${FAILED_COUNT}" -eq 0 ]; then
    echo -e "${GREEN}All integration tests passed successfully!${NC}"
    exit 0
else
    echo -e "${RED}Some integration tests failed. Please check the logs above.${NC}"
    exit 1
fi
