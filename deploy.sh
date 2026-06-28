#!/usr/bin/env bash
set -euo pipefail

# Directory this script lives in, so source files are found regardless of CWD.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

MYTHIC_DIR="${SCRIPT_DIR}/../Mythic"
BUILD=false

usage() {
    echo "Usage: $0 [-d|--mythic-dir <path>] [-b|--build]"
    echo ""
    echo "  -d, --mythic-dir <path>   Path to the Mythic directory (default: ${MYTHIC_DIR})"
    echo "  -b, --build               Build mythic-cli ('make local') after copying the files"
    echo "  -h, --help                Show this help message"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        -d|--mythic-dir)
            MYTHIC_DIR="$2"
            shift 2
            ;;
        -b|--build)
            BUILD=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

if [[ ! -d "${MYTHIC_DIR}" ]]; then
    echo "Error: Mythic directory not found: ${MYTHIC_DIR}" >&2
    exit 1
fi

CLI_CMD="${MYTHIC_DIR}/Mythic_CLI/src/cmd"

cp -r "${SCRIPT_DIR}/cmd/"* "${CLI_CMD}/"

if [[ "${BUILD}" == true ]]; then
    cd "${MYTHIC_DIR}"
    make local
fi
