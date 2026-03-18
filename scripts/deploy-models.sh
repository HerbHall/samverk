#!/bin/bash
# Deploy Ollama models to all configured hosts.
# Reads model assignments from a manifest file.
# Usage: ./scripts/deploy-models.sh [--check|--pull]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST="${SCRIPT_DIR}/model-manifest.yaml"

# Colors (disable if not a terminal).
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BOLD='\033[1m'
    RESET='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BOLD=''
    RESET=''
fi

usage() {
    echo "Usage: $0 [--check|--pull]"
    echo ""
    echo "  --check   Compare current models on each host vs manifest, report drift"
    echo "  --pull    Pull missing models on each host via Ollama API"
    echo ""
    echo "Reads model assignments from: ${MANIFEST}"
    exit 1
}

if [ $# -ne 1 ]; then
    usage
fi

MODE="$1"
case "${MODE}" in
    --check|--pull) ;;
    *) usage ;;
esac

if [ ! -f "${MANIFEST}" ]; then
    echo -e "${RED}Error: manifest not found: ${MANIFEST}${RESET}"
    exit 1
fi

# Parse YAML manifest without external dependencies.
# Extracts host entries as: hostname url model1,model2,...
parse_manifest() {
    local current_host=""
    local current_url=""
    local current_models=""

    while IFS= read -r line; do
        # Skip comments and empty lines.
        [[ "${line}" =~ ^[[:space:]]*# ]] && continue
        [[ -z "${line// /}" ]] && continue

        # Match host name (e.g., "  ollama-nzxt:").
        if [[ "${line}" =~ ^[[:space:]]{2}([a-zA-Z0-9_-]+):[[:space:]]*$ ]]; then
            # Emit previous host if any.
            if [ -n "${current_host}" ] && [ -n "${current_url}" ]; then
                echo "${current_host}|${current_url}|${current_models}"
            fi
            current_host="${BASH_REMATCH[1]}"
            current_url=""
            current_models=""
            continue
        fi

        # Match url field.
        if [[ "${line}" =~ ^[[:space:]]+url:[[:space:]]*(.+)$ ]]; then
            current_url="${BASH_REMATCH[1]}"
            continue
        fi

        # Match model entry (e.g., "      - qwen2.5-coder:7b").
        if [[ "${line}" =~ ^[[:space:]]+-[[:space:]]+(.+)$ ]]; then
            local model="${BASH_REMATCH[1]}"
            if [ -n "${current_models}" ]; then
                current_models="${current_models},${model}"
            else
                current_models="${model}"
            fi
            continue
        fi
    done < "${MANIFEST}"

    # Emit last host.
    if [ -n "${current_host}" ] && [ -n "${current_url}" ]; then
        echo "${current_host}|${current_url}|${current_models}"
    fi
}

# Get list of models currently on a host via Ollama API.
get_remote_models() {
    local url="$1"
    local response
    response=$(curl -s --connect-timeout 5 --max-time 10 "${url}/api/tags" 2>/dev/null) || {
        echo "ERROR"
        return
    }

    # Extract model names from JSON response using simple pattern matching.
    # Ollama returns {"models":[{"name":"model:tag",...},...]}
    echo "${response}" | tr ',' '\n' | grep -o '"name":"[^"]*"' | sed 's/"name":"//;s/"//' || true
}

# Pull a model on a remote host via Ollama API.
pull_model() {
    local url="$1"
    local model="$2"

    echo -e "  ${YELLOW}Pulling ${model}...${RESET}"
    local status_code
    status_code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 --max-time 600 \
        -X POST "${url}/api/pull" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"${model}\",\"stream\":false}" 2>/dev/null) || {
        echo -e "  ${RED}Failed to pull ${model} (connection error)${RESET}"
        return 1
    }

    if [ "${status_code}" = "200" ]; then
        echo -e "  ${GREEN}Pulled ${model} successfully${RESET}"
        return 0
    else
        echo -e "  ${RED}Failed to pull ${model} (HTTP ${status_code})${RESET}"
        return 1
    fi
}

drift_found=0

while IFS='|' read -r host_name host_url host_models; do
    echo -e "${BOLD}${host_name}${RESET} (${host_url})"

    # Get remote models.
    remote_models=$(get_remote_models "${host_url}")
    if [ "${remote_models}" = "ERROR" ]; then
        echo -e "  ${RED}Unreachable${RESET}"
        drift_found=1
        echo ""
        continue
    fi

    # Check each expected model.
    IFS=',' read -ra expected_models <<< "${host_models}"
    for model in "${expected_models[@]}"; do
        if echo "${remote_models}" | grep -qF "${model}"; then
            echo -e "  ${GREEN}OK${RESET}  ${model}"
        else
            echo -e "  ${RED}MISSING${RESET}  ${model}"
            drift_found=1

            if [ "${MODE}" = "--pull" ]; then
                pull_model "${host_url}" "${model}" || true
            fi
        fi
    done
    echo ""
done < <(parse_manifest)

if [ "${MODE}" = "--check" ] && [ "${drift_found}" -ne 0 ]; then
    echo -e "${RED}Drift detected. Run with --pull to fix.${RESET}"
    exit 1
fi

if [ "${MODE}" = "--check" ] && [ "${drift_found}" -eq 0 ]; then
    echo -e "${GREEN}All hosts match manifest.${RESET}"
fi
