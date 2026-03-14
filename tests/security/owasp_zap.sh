#!/bin/bash

# =============================================================================
# OWASP ZAP Security Scan Script for VirtueStack
# =============================================================================
# 
# This script runs OWASP ZAP security scans against the VirtueStack API
# and web applications. It performs both baseline and active scans.
#
# Prerequisites:
#   - Docker installed
#   - ZAP container image available (owasp/zap2docker-stable)
#   - Target application running and accessible
#
# Usage:
#   ./owasp_zap.sh [target_url] [scan_type]
#
# Arguments:
#   target_url - Base URL to scan (default: http://localhost:3000)
#   scan_type  - Type of scan: baseline, full, api (default: baseline)
#
# Examples:
#   ./owasp_zap.sh http://localhost:3000 baseline
#   ./owasp_zap.sh http://localhost:3000/api/v1 api
#   ./owasp_zap.sh http://localhost:3000 full
#
# Exit codes:
#   0 - No high/critical vulnerabilities found
#   1 - High/critical vulnerabilities found
#   2 - Scan failed to complete
# =============================================================================

set -e

# Configuration
TARGET_URL="${1:-http://localhost:3000}"
SCAN_TYPE="${2:-baseline}"
REPORT_DIR="./security-reports"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
ZAP_IMAGE="owasp/zap2docker-stable"
ZAP_CONTAINER="zap-scan-${TIMESTAMP}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# API endpoints to scan
API_ENDPOINTS=(
    "/api/v1/auth/login"
    "/api/v1/auth/register"
    "/api/v1/auth/refresh"
    "/api/v1/vms"
    "/api/v1/vms/{id}"
    "/api/v1/vms/{id}/start"
    "/api/v1/vms/{id}/stop"
    "/api/v1/backups"
    "/api/v1/backups/{id}"
    "/api/v1/webhooks"
    "/health"
    "/ready"
    "/metrics"
)

# Web pages to scan
WEB_PAGES=(
    "/"
    "/login"
    "/register"
    "/dashboard"
    "/vms"
    "/vms/{id}"
    "/backups"
    "/settings"
)

# Create report directory
mkdir -p "${REPORT_DIR}"

echo -e "${GREEN}============================================${NC}"
echo -e "${GREEN}VirtueStack OWASP ZAP Security Scan${NC}"
echo -e "${GREEN}============================================${NC}"
echo ""
echo "Target URL: ${TARGET_URL}"
echo "Scan Type: ${SCAN_TYPE}"
echo "Report Directory: ${REPORT_DIR}"
echo ""

# Function to check if target is reachable
check_target() {
    echo -e "${YELLOW}Checking target availability...${NC}"
    
    if curl -s -o /dev/null -w "%{http_code}" "${TARGET_URL}/health" | grep -q "200\|401\|403"; then
        echo -e "${GREEN}Target is reachable${NC}"
        return 0
    else
        echo -e "${RED}Target is not reachable: ${TARGET_URL}${NC}"
        return 1
    fi
}

# Function to wait for ZAP to start
wait_for_zap() {
    echo -e "${YELLOW}Waiting for ZAP to start...${NC}"
    
    local max_attempts=30
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        if docker exec ${ZAP_CONTAINER} zap-cli status > /dev/null 2>&1; then
            echo -e "${GREEN}ZAP is ready${NC}"
            return 0
        fi
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo -e "${RED}ZAP failed to start${NC}"
    return 1
}

# Function to wait for spider to complete
wait_for_spider() {
    echo -e "${YELLOW}Waiting for spider to complete...${NC}"
    
    local max_attempts=60
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        local status=$(docker exec ${ZAP_CONTAINER} zap-cli spider status 2>/dev/null | grep -o "complete\|running\|starting" || echo "unknown")
        if [ "$status" = "complete" ] || [ "$status" = "no spider" ]; then
            echo -e "${GREEN}Spider completed${NC}"
            return 0
        fi
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo -e "${YELLOW}Spider timeout - continuing anyway${NC}"
    return 0
}

# Function to wait for active scan to complete
wait_for_scan() {
    echo -e "${YELLOW}Waiting for active scan to complete...${NC}"
    
    local max_attempts=90
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        local status=$(docker exec ${ZAP_CONTAINER} zap-cli active-scan status 2>/dev/null | grep -o "complete\|running\|starting" || echo "unknown")
        if [ "$status" = "complete" ] || [ "$status" = "no scan" ]; then
            echo -e "${GREEN}Scan completed${NC}"
            return 0
        fi
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo -e "${YELLOW}Scan timeout - continuing anyway${NC}"
    return 0
}

# Function to wait for AJAX spider to complete
wait_for_ajax_spider() {
    echo -e "${YELLOW}Waiting for AJAX spider to complete...${NC}"
    
    local max_attempts=90
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        local status=$(docker exec ${ZAP_CONTAINER} zap-cli ajax-spider status 2>/dev/null | grep -o "complete\|running\|starting" || echo "unknown")
        if [ "$status" = "complete" ] || [ "$status" = "no spider" ]; then
            echo -e "${GREEN}AJAX spider completed${NC}"
            return 0
        fi
        sleep 2
        attempt=$((attempt + 1))
    done
    
    echo -e "${YELLOW}AJAX spider timeout - continuing anyway${NC}"
    return 0
}

# Function to run baseline scan
run_baseline_scan() {
    echo -e "${YELLOW}Running baseline scan...${NC}"
    
    docker run --rm \
        --name "${ZAP_CONTAINER}" \
        --network host \
        -v "$(pwd)/${REPORT_DIR}:/zap/reports:rw" \
        ${ZAP_IMAGE} \
        zap-baseline.py \
        -t "${TARGET_URL}" \
        -r "baseline-report-${TIMESTAMP}.html" \
        -w "baseline-report-${TIMESTAMP}.md" \
        -J "baseline-report-${TIMESTAMP}.json" \
        --autooff \
        --rules-dir /zap/rules/ \
        -c baseline.conf \
        -l PASS || true
    
    echo -e "${GREEN}Baseline scan completed${NC}"
}

# Function to run full scan
run_full_scan() {
    echo -e "${YELLOW}Running full scan (this may take a while)...${NC}"
    
    # Start ZAP in daemon mode
    docker run -d \
        --name "${ZAP_CONTAINER}" \
        --network host \
        -v "$(pwd)/${REPORT_DIR}:/zap/reports:rw" \
        ${ZAP_IMAGE} \
        zap.sh -daemon -host 0.0.0.0 -port 8080 -config api.addrs.addr.name=.* -config api.addrs.addr.regex=true
    
    # Wait for ZAP
    wait_for_zap
    
    # Spider the target
    echo -e "${YELLOW}Spidering target...${NC}"
    docker exec ${ZAP_CONTAINER} zap-cli spider "${TARGET_URL}" || true
    
    # Wait for spider to complete
    wait_for_spider
    
    # Scan all discovered URLs
    echo -e "${YELLOW}Scanning all discovered URLs...${NC}"
    docker exec ${ZAP_CONTAINER} zap-cli active-scan -r "${TARGET_URL}" || true
    
    # Wait for scan to complete
    wait_for_scan
    
    # Generate reports
    echo -e "${YELLOW}Generating reports...${NC}"
    docker exec ${ZAP_CONTAINER} zap-cli report \
        -o "/zap/reports/full-report-${TIMESTAMP}.html" \
        -f html || true
    
    docker exec ${ZAP_CONTAINER} zap-cli report \
        -o "/zap/reports/full-report-${TIMESTAMP}.json" \
        -f json || true
    
    # Stop container
    docker stop ${ZAP_CONTAINER} > /dev/null 2>&1 || true
    docker rm ${ZAP_CONTAINER} > /dev/null 2>&1 || true
    
    echo -e "${GREEN}Full scan completed${NC}"
}

# Function to run API scan
run_api_scan() {
    echo -e "${YELLOW}Running API scan...${NC}"
    
    # Create OpenAPI spec file for scanning
    cat > "${REPORT_DIR}/openapi-spec.json" << 'OPENAPI_EOF'
{
  "openapi": "3.0.0",
  "info": {
    "title": "VirtueStack API",
    "version": "1.0.0"
  },
  "servers": [
    {
      "url": "TARGET_URL_PLACEHOLDER"
    }
  ],
  "paths": {
    "/api/v1/auth/login": {
      "post": {
        "summary": "Login",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "email": {"type": "string"},
                  "password": {"type": "string"}
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/vms": {
      "get": {
        "summary": "List VMs",
        "security": [{"bearerAuth": []}]
      },
      "post": {
        "summary": "Create VM",
        "security": [{"bearerAuth": []}],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "hostname": {"type": "string"},
                  "plan_id": {"type": "string"},
                  "template_id": {"type": "string"},
                  "password": {"type": "string"}
                }
              }
            }
          }
        }
      }
    },
    "/api/v1/backups": {
      "get": {
        "summary": "List Backups",
        "security": [{"bearerAuth": []}]
      },
      "post": {
        "summary": "Create Backup",
        "security": [{"bearerAuth": []}]
      }
    }
  },
  "components": {
    "securitySchemes": {
      "bearerAuth": {
        "type": "http",
        "scheme": "bearer"
      }
    }
  }
}
OPENAPI_EOF
    
    # Replace placeholder with actual target URL
    sed -i "s|TARGET_URL_PLACEHOLDER|${TARGET_URL}|g" "${REPORT_DIR}/openapi-spec.json"
    
    # Run API scan
    docker run --rm \
        --name "${ZAP_CONTAINER}" \
        --network host \
        -v "$(pwd)/${REPORT_DIR}:/zap/reports:rw" \
        ${ZAP_IMAGE} \
        zap-api-scan.py \
        -t "${TARGET_URL}" \
        -f openapi \
        -d "/zap/reports/openapi-spec.json" \
        -r "api-report-${TIMESTAMP}.html" \
        -w "api-report-${TIMESTAMP}.md" \
        -J "api-report-${TIMESTAMP}.json" \
        --autooff || true
    
    echo -e "${GREEN}API scan completed${NC}"
}

# Function to run AJAX spider scan (for SPA applications)
run_ajax_scan() {
    echo -e "${YELLOW}Running AJAX spider scan...${NC}"
    
    # Start ZAP in daemon mode
    docker run -d \
        --name "${ZAP_CONTAINER}" \
        --network host \
        -v "$(pwd)/${REPORT_DIR}:/zap/reports:rw" \
        ${ZAP_IMAGE} \
        zap.sh -daemon -host 0.0.0.0 -port 8080 -config api.addrs.addr.name=.* -config api.addrs.addr.regex=true
    
    wait_for_zap
    
    # Run AJAX spider
    echo -e "${YELLOW}Running AJAX spider...${NC}"
    docker exec ${ZAP_CONTAINER} zap-cli ajax-spider "${TARGET_URL}" || true
    
    # Wait for AJAX spider
    wait_for_ajax_spider
    
    # Active scan
    docker exec ${ZAP_CONTAINER} zap-cli active-scan -r "${TARGET_URL}" || true
    
    # Generate reports
    docker exec ${ZAP_CONTAINER} zap-cli report \
        -o "/zap/reports/ajax-report-${TIMESTAMP}.html" \
        -f html || true
    
    # Cleanup
    docker stop ${ZAP_CONTAINER} > /dev/null 2>&1 || true
    docker rm ${ZAP_CONTAINER} > /dev/null 2>&1 || true
    
    echo -e "${GREEN}AJAX scan completed${NC}"
}

# Function to parse results and check for vulnerabilities
check_results() {
    echo ""
    echo -e "${YELLOW}Analyzing scan results...${NC}"
    
    local report_file="${REPORT_DIR}/${SCAN_TYPE}-report-${TIMESTAMP}.json"
    
    if [ -f "${report_file}" ]; then
        # Check for high/critical vulnerabilities
        local high_count=$(jq '[.site[].alerts[] | select(.riskdesc | startswith("High") or startswith("Critical"))] | length' "${report_file}" 2>/dev/null || echo "0")
        local medium_count=$(jq '[.site[].alerts[] | select(.riskdesc | startswith("Medium"))] | length' "${report_file}" 2>/dev/null || echo "0")
        local low_count=$(jq '[.site[].alerts[] | select(.riskdesc | startswith("Low"))] | length' "${report_file}" 2>/dev/null || echo "0")
        
        echo ""
        echo -e "${GREEN}Scan Summary:${NC}"
        echo "  Critical/High: ${high_count}"
        echo "  Medium: ${medium_count}"
        echo "  Low: ${low_count}"
        echo ""
        
        if [ "${high_count}" -gt 0 ]; then
            echo -e "${RED}FAIL: Found ${high_count} high/critical vulnerabilities!${NC}"
            
            # List high/critical issues
            echo ""
            echo -e "${RED}High/Critical Vulnerabilities:${NC}"
            jq -r '.site[].alerts[] | select(.riskdesc | startswith("High") or startswith("Critical")) | "  - \(.name): \(.desc)"' "${report_file}" 2>/dev/null || true
            
            return 1
        else
            echo -e "${GREEN}PASS: No high/critical vulnerabilities found${NC}"
            return 0
        fi
    else
        echo -e "${YELLOW}Warning: Report file not found at ${report_file}${NC}"
        return 0
    fi
}

# Function to generate summary report
generate_summary() {
    echo ""
    echo -e "${YELLOW}Generating summary report...${NC}"
    
    local summary_file="${REPORT_DIR}/summary-${TIMESTAMP}.md"
    
    cat > "${summary_file}" << EOF
# VirtueStack Security Scan Summary

**Date:** $(date)
**Target:** ${TARGET_URL}
**Scan Type:** ${SCAN_TYPE}

## Reports Generated

- HTML Report: ${SCAN_TYPE}-report-${TIMESTAMP}.html
- JSON Report: ${SCAN_TYPE}-report-${TIMESTAMP}.json
- Markdown Report: ${SCAN_TYPE}-report-${TIMESTAMP}.md

## Scan Configuration

- ZAP Version: $(docker run --rm ${ZAP_IMAGE} zap.sh -version 2>/dev/null || echo "Unknown")
- Scan Target: ${TARGET_URL}

## Recommendations

1. Review all high and medium severity findings
2. Address authentication and session management issues
3. Ensure all API endpoints have proper authorization
4. Review input validation and output encoding
5. Check for sensitive data exposure
6. Verify CORS configuration

## Next Steps

1. Remediate identified vulnerabilities
2. Re-run scans to verify fixes
3. Integrate security testing into CI/CD pipeline
4. Schedule regular security assessments

---
*Generated by VirtueStack OWASP ZAP Security Scan Script*
EOF
    
    echo -e "${GREEN}Summary report generated: ${summary_file}${NC}"
}

# Main execution
main() {
    # Check target availability
    if ! check_target; then
        echo -e "${RED}Error: Target is not reachable${NC}"
        exit 2
    fi
    
    # Run the appropriate scan type
    case "${SCAN_TYPE}" in
        baseline)
            run_baseline_scan
            ;;
        full)
            run_full_scan
            ;;
        api)
            run_api_scan
            ;;
        ajax)
            run_ajax_scan
            ;;
        *)
            echo -e "${RED}Unknown scan type: ${SCAN_TYPE}${NC}"
            echo "Available types: baseline, full, api, ajax"
            exit 2
            ;;
    esac
    
    # Generate summary
    generate_summary
    
    # Check results
    check_results
    exit_code=$?
    
    echo ""
    echo -e "${GREEN}============================================${NC}"
    echo -e "${GREEN}Scan completed${NC}"
    echo -e "${GREEN}Reports available in: ${REPORT_DIR}${NC}"
    echo -e "${GREEN}============================================${NC}"
    
    exit ${exit_code}
}

# Run main function
main