#!/bin/sh
# Prepare the data directory and reload systemd. The service is intentionally
# NOT enabled or started: the operator must provide a private key first
# (see /etc/teled/teled.env) and then run `systemctl enable --now teled`.
set -e

mkdir -p /var/lib/teled/objects
chown -R teled:teled /var/lib/teled
chmod 0750 /var/lib/teled

if command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi
