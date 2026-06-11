#!/bin/sh
# Create the system user and group for teled before files are unpacked,
# so packaged files can be owned by it.
set -e

if ! getent group teled >/dev/null 2>&1; then
	groupadd --system teled 2>/dev/null \
		|| addgroup -S teled 2>/dev/null \
		|| true
fi

if ! getent passwd teled >/dev/null 2>&1; then
	useradd --system --gid teled --home-dir /var/lib/teled \
		--no-create-home --shell /usr/sbin/nologin \
		--comment "teled server" teled 2>/dev/null \
		|| adduser -S -D -H -G teled -h /var/lib/teled -s /sbin/nologin teled 2>/dev/null \
		|| true
fi
