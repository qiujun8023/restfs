# restfs

A lightweight HTTP file server. Reads are public and unauthenticated; writes (upload, overwrite, delete) are protected by a single `ADMIN_TOKEN`.

Ideal for distributing build artifacts, firmware images, or as a simple file host.

## Features

- **Public reads** — browsing directories and downloading files requires no authentication
- **Directory listing** — browser-friendly HTML with breadcrumb navigation; returns JSON for API clients via `Accept: application/json`
- **Markdown rendering** — a `README.md` in any directory is automatically rendered below the file listing
- **Token-protected writes** — `PUT`, `POST`, and `DELETE` require `Authorization: Bearer <ADMIN_TOKEN>`
- **Auto mkdir** — parent directories are created automatically on upload (both PUT stream and POST multipart)
- **Empty dir pruning** — after a file is deleted, empty parent directories are cleaned up automatically
- **Shell-compatible** — files can be managed directly on the host via `cp`, `rsync`, or `rm`; changes take effect immediately
- **Lightweight Docker image** — Alpine-based single binary, ~10 MB

## HTTP API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/<path>` | No | Download a file or list a directory |
| `PUT` | `/<path>` | Yes | Upload a file via raw body stream. Path must include the filename. |
| `POST` | `/<path>/` | Yes | Upload a file via multipart form (field name: `file`) |
| `DELETE` | `/<path>` | Yes | Delete a file (directories cannot be deleted) |

Authentication uses the HTTP header: `Authorization: Bearer <ADMIN_TOKEN>`

### Content negotiation (GET on a directory)

| `Accept` header | Response |
|-----------------|----------|
| `text/html` (browser default) | HTML directory page |
| `application/json` | JSON array of entries |

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
| `400` | Bad request (invalid path, path traversal, attempted directory delete, etc.) |
| `401` | Missing or invalid token |
| `404` | File or directory not found |

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_TOKEN` | **required** | Token for write and delete operations |
| `DATA_DIR` | `/data` | Root directory for stored files |
| `PORT` | `8080` | Port the server listens on |

## Quick start

**Docker:**

```bash
docker run -d \
  -p 8080:8080 \
  -e ADMIN_TOKEN=your-secret-token \
  -v $(pwd)/data:/data \
  qiujun8023/restfs:latest
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
# HTML directory listing (browser-friendly)
curl http://localhost:8080/firmware/

# JSON listing for scripts
curl -H "Accept: application/json" http://localhost:8080/firmware/

# Download a file
curl -O http://localhost:8080/firmware/v1.0.0/firmware.bin
```

### Delete

```bash
curl -X DELETE \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/firmware/v0.9.0/old.bin
```
