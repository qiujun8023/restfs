# restfs

[English](README.md)

一个轻量级 HTTP 文件服务器。读操作公开无需认证；写操作（上传、覆盖、删除）通过单一 `ADMIN_TOKEN` 保护。

适合分发构建产物、固件镜像，或作为简单的文件托管服务。

## 功能

- **公开读取** — 浏览目录和下载文件无需认证
- **目录列表** — 浏览器友好的 HTML 页面，含面包屑导航；API 客户端通过 `Accept: application/json` 获取 JSON
- **Markdown 渲染** — 任意目录下的 `README.md` 自动渲染在文件列表下方
- **Token 保护写操作** — `PUT`、`POST`、`DELETE` 需要 `Authorization: Bearer <ADMIN_TOKEN>`
- **自动创建目录** — 上传时自动创建不存在的父级目录（PUT 流式上传和 POST 表单上传均支持）
- **空目录清理** — 删除文件后，自动向上清理空父级目录
- **Shell 兼容** — 可直接在宿主机通过 `cp`、`rsync`、`rm` 管理文件，修改即时生效
- **轻量 Docker 镜像** — 基于 Alpine 的单一二进制，约 10 MB
- **断点续传** — 支持 HTTP Range 请求（206 Partial Content）

## HTTP API

| 方法 | 路径 | 需要认证 | 说明 |
|------|------|----------|------|
| `GET` | `/<path>` | 否 | 下载文件或列出目录 |
| `PUT` | `/<path>` | 是 | 通过请求体流式上传文件，路径须包含文件名 |
| `POST` | `/<path>/` | 是 | 通过 multipart 表单上传文件（字段名：`file`） |
| `DELETE` | `/<path>` | 是 | 删除文件（不支持删除目录） |

认证方式：HTTP 请求头 `Authorization: Bearer <ADMIN_TOKEN>`

### 内容协商（GET 目录）

| `Accept` 请求头 | 响应格式 |
|-----------------|----------|
| `text/html`（浏览器默认） | HTML 目录页面 |
| `application/json` | JSON 数组 |

### JSON 格式

**目录列表**（`GET` 携带 `Accept: application/json`）— 返回平铺数组，目录优先：

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

**写操作响应**（`PUT` / `POST`）— 返回已上传文件的元数据：

```json
{
  "name": "firmware.bin",
  "type": "file",
  "size": 524288,
  "path": "/firmware/v1.0.0/firmware.bin",
  "modified": "2024-01-01T00:00:00Z"
}
```

### 状态码

| 状态码 | 含义 |
|--------|------|
| `200` | 成功（读取，或覆盖已有文件） |
| `201` | 已创建（新文件） |
| `204` | 已删除 |
| `400` | 请求错误（路径无效、路径穿越、尝试删除目录等） |
| `401` | Token 缺失或错误 |
| `404` | 文件或目录不存在 |

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `ADMIN_TOKEN` | **必填** | 写操作和删除操作的认证 Token |
| `DATA_DIR` | `/data` | 存储文件的根目录 |
| `PORT` | `8080` | 服务监听端口 |

## 快速开始

**Docker：**

```bash
docker run -d \
  -p 8080:8080 \
  -e ADMIN_TOKEN=your-secret-token \
  -v $(pwd)/data:/data \
  qiujun8023/restfs:latest
```

> 挂载的数据目录需对容器用户（UID 1000）有写权限。

**Docker Compose：**

```bash
ADMIN_TOKEN=your-secret-token docker compose up -d
```

**从源码构建：**

```bash
go build -o restfs .
ADMIN_TOKEN=your-secret-token DATA_DIR=/tmp/restfs ./restfs
```

## 示例

### 上传

```bash
# PUT：直接流式上传（路径须包含文件名）
curl -X PUT \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  --data-binary @firmware.bin \
  http://localhost:8080/firmware/v1.0.0/firmware.bin

# POST：multipart 表单上传（路径为目标目录）
curl -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "file=@app.zip" \
  http://localhost:8080/releases/v1.0.0/
```

### 浏览与下载

```bash
# HTML 目录列表（浏览器友好）
curl http://localhost:8080/firmware/

# JSON 列表（脚本使用）
curl -H "Accept: application/json" http://localhost:8080/firmware/

# 下载文件
curl -O http://localhost:8080/firmware/v1.0.0/firmware.bin

# 断点续传
curl -C - -O http://localhost:8080/firmware/v1.0.0/firmware.bin
```

### 删除

```bash
curl -X DELETE \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/firmware/v0.9.0/old.bin
```
