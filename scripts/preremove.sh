#!/bin/sh
# Stop and disable the service before removal, if it was ever enabled.
set -e

if command -v systemctl >/dev/null 2>&1; then
	systemctl stop teled.service >/dev/null 2>&1 || true
	systemctl disable teled.service >/dev/null 2>&1 || true
fi
