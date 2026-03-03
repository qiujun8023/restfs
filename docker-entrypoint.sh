#!/bin/sh
set -e

PUID=${PUID:-1000}
PGID=${PGID:-1000}

addgroup -g "$PGID" app
adduser -D -u "$PUID" -G app -s /sbin/nologin app
chown app:app /data

exec su-exec app "$@"
