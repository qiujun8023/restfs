#!/bin/sh
set -e

PUID=${PUID:-1000}
PGID=${PGID:-1000}

getent group app >/dev/null 2>&1 || addgroup -g "$PGID" app
id app >/dev/null 2>&1 || adduser -D -u "$PUID" -G app -s /sbin/nologin app
chown app:app /data

exec su-exec app "$@"
