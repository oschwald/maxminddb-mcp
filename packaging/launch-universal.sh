#!/bin/sh
set -eu

platform=$(uname -s)/$(uname -m)
case "$platform" in
    Darwin/x86_64) target=darwin_amd64 ;;
    Darwin/arm64) target=darwin_arm64 ;;
    Linux/x86_64) target=linux_amd64 ;;
    Linux/aarch64|Linux/arm64) target=linux_arm64 ;;
    *)
        printf 'Unsupported platform: %s. Use a standalone release for your platform.\n' "$platform" >&2
        exit 1
        ;;
esac

exec "${0%/*}/$target/maxminddb-mcp" "$@"
