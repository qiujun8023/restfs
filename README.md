# restfs

[中文](README.zh.md)

A lightweight HTTP file server. File downloads and `index.html` pages are public; directory listing requires Basic Auth by default (optional public toggle via `PUBLIC_DIR_LIST=true`); writes (upload, overwrite, delete) are protected by a single `ADMIN_TOKEN`.

Ideal for distributing build artifacts, firmware images, or as a simple file host.

## Features

- **Public file downloads** — downloading files and serving `index.html` pages requires no authentication
- **Optional public directory listing** — directory listing requires Basic Auth (`admin` / `ADMIN_TOKEN`) by default, and can be opened publicly with `PUBLIC_DIR_LIST=true`
- **CORS enabled by default** — public reads and authenticated requests can be called from browsers on other origins
- **Directory listing** — browser-friendly HTML with breadcrumb navigation; returns JSON for API clients via `Accept: application/json`
- **Index page support** — if a directory contains an `index.html`, it is served publicly instead of the listing for HTML requests; JSON requests still return the listing (subject to `PUBLIC_DIR_LIST`)
- **Markdown rendering** — a `README.md` in any directory is automatically rendered below the file listing; the filename is case-insensitive and raw HTML in it is escaped
- **Token-protected writes** — `PUT`, `POST`, and `DELETE` require `Authorization: Bearer <ADMIN_TOKEN>`
- **Auto mkdir** — parent directories are created automatically on upload (both PUT stream and POST multipart)
- **Empty dir pruning** — after a file is deleted, empty parent directories are cleaned up automatically
- **Shell-compatible** — files can be managed directly on the host via `cp`, `rsync`, or `rm`; changes take effect immediately
- **Lightweight Docker image** — Alpine-based single binary, ~10 MB
- **Resumable downloads** — supports HTTP Range requests (`206 Partial Content`)

## HTTP API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/<path>` | Files & `index.html`: No; Directory listing: Yes when `PUBLIC_DIR_LIST=false` | Download a file or list a directory |
| `PUT` | `/<path>` | Yes | Upload a file via raw body stream. Path must include the filename. |
| `POST` | `/<path>/` | Yes | Upload a file via multipart form (field name: `file`) |
| `DELETE` | `/<path>` | Yes | Delete a file (directories cannot be deleted) |

- Write authentication uses the HTTP header: `Authorization: Bearer <ADMIN_TOKEN>`
- Directory listing authentication (when `PUBLIC_DIR_LIST=false`) prompts for HTTP Basic Auth with username `admin` and password `<ADMIN_TOKEN>` (also accepts `Authorization: Bearer <ADMIN_TOKEN>`)

### CORS

CORS is enabled by default for all routes. The server allows any origin (`Access-Control-Allow-Origin: *`), supports browser preflight requests, and exposes range/download headers such as `Content-Range`, `Accept-Ranges`, `ETag`, and `Last-Modified`.

Authenticated requests (writes, and directory listings when `PUBLIC_DIR_LIST=false`) still require the `Authorization` header.

### Content negotiation (GET on a directory)

| Request | Response |
|---------|----------|
| `?format=json` or `Accept: application/json` | JSON array of entries |
| `Accept: text/html` (browser default) | HTML directory page, or `index.html` if present |
| Anything else (e.g. curl's default `*/*`) | Same as `text/html` |

Directory paths without a trailing slash are redirected with `301` to the canonical trailing-slash form for HTML requests, so relative links in `index.html` resolve correctly. JSON requests (`?format=json` or `Accept: application/json`) are served in place without redirect.

### JSON formats

**Directory listing** (`GET` with `Accept: application/json`) — returns a flat array, directories first:

```json
[
  {
    "name": "subdir/",
    "type": "directory",
    "path": "/path/to/subdir/",
    "modified": "2024-01-01T00:00:00Z"
  },
  {
    "name": "file.txt",
    "type": "file",
    "size": 1024,
    "path": "/path/to/file.txt",
    "modified": "2024-01-01T00:00:00Z"
  }
]
```

**Write response** (`PUT` / `POST`) — returns the metadata of the uploaded file:

```json
{
  "name": "firmware.bin",
  "type": "file",
  "size": 524288,
  "path": "/firmware/v1.0.0/firmware.bin",
  "modified": "2024-01-01T00:00:00Z"
}
```

### Status codes

| Code | Meaning |
|------|---------|
| `200` | Success (read, or overwriting an existing file) |
| `201` | Created (new file) |
| `204` | Deleted |
| `301` | Directory path redirected to trailing-slash form |
| `400` | Bad request (invalid path, path traversal, attempted directory delete, etc.) |
| `401` | Missing or invalid credentials (Bearer token or Basic Auth) |
| `404` | File or directory not found |

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_TOKEN` | **required** | Token for write/delete operations and Basic Auth password for directory listing |
| `PUBLIC_DIR_LIST` | `false` | Allow unauthenticated directory listing when `true`; when `false`, directory listing requires Basic Auth (`admin` / `ADMIN_TOKEN`) |
| `DATA_DIR` | `/data` | Root directory for stored files |
| `PORT` | `8080` | Port the server listens on |
| `PUID` | `1000` | UID of the user that owns and runs the process |
| `PGID` | `1000` | GID of the user that owns and runs the process |

## Quick start

**Docker:**

```bash
docker run -d \
  -p 8080:8080 \
  -e ADMIN_TOKEN=your-secret-token \
  -e PUID=$(id -u) \
  -e PGID=$(id -g) \
  -v $(pwd)/data:/data \
  ghcr.io/qiujun8023/restfs:latest
```

**Docker Compose:**

```bash
ADMIN_TOKEN=your-secret-token docker compose up -d
```

**From source:**

```bash
go build -o restfs .
ADMIN_TOKEN=your-secret-token DATA_DIR=/tmp/restfs ./restfs
```

## Examples

### Upload

```bash
# PUT: stream a file directly (path must include the filename)
curl -X PUT \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  --data-binary @firmware.bin \
  http://localhost:8080/firmware/v1.0.0/firmware.bin

# POST: multipart form upload (path is the target directory)
curl -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "file=@app.zip" \
  http://localhost:8080/releases/v1.0.0/
```

### Browse & download

```bash
# HTML directory listing (Basic Auth required unless PUBLIC_DIR_LIST=true)
curl -u "admin:$ADMIN_TOKEN" http://localhost:8080/firmware/

# JSON listing for scripts
curl -u "admin:$ADMIN_TOKEN" -H "Accept: application/json" http://localhost:8080/firmware/

# Download a file (public, no auth required)
curl -O http://localhost:8080/firmware/v1.0.0/firmware.bin
```

### Delete

```bash
curl -X DELETE \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/firmware/v0.9.0/old.bin
```
