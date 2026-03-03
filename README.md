# restfs

一个轻量级的 HTTP 文件服务器。支持无鉴权公开读取，写入（上传、覆盖、删除）统一通过 `ADMIN_TOKEN` 鉴权。非常适合用来分发构建产物、固件或用作简易图床。

## 特性

- **公开读取** — 浏览目录或下载文件无需鉴权，可被全球访问。
- **目录查看** — 浏览器友好：现代化且清爽的 HTML 目录页（类似 Nginx 的 autoindex）。对 API 客户端支持 `Accept: application/json` 返回 JSON。
- **内置 Markdown 渲染** — 如果目录下存在 `README.md`，会自动在文件列表下方渲染它的内容。
- **Token 保护** — 写入操作（`PUT`, `POST`, `DELETE`）需要通过 HTTP Header: `Authorization: Bearer <ADMIN_TOKEN>` 鉴权。
- **自动创建目录** — 上传文件时，如果父级不存在会自动 `mkdir -p` 创建（支持 PUT 原始流或 POST 表单）。
- **同步空目录清理** — 删除最后一个文件后，会自动将其所在的空目录连带逐级清理。
- **Shell 友好与无状态** — 通过 `docker -v` 挂载目录后，你可以直接使用 `cp`、`rsync` 或 `rm` 操作宿主机的资源，前端会实时生效。
- **多平台 Docker 支持** — 基于 Alpine 构建的单文件镜像，体积仅 ~10MB。

---

## HTTP API

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|-------------|
| `GET` | `/<path>` | ✗ | 读文件 或 列出目录内容 |
| `PUT` | `/<path>` | ✓ | 上传文件（请求体即二进制文件流），推荐 CI 使用 |
| `POST` | `/<path>/` | ✓ | 以表单方式上传文件（字段名必须为 `file`） |
| `DELETE` | `/<path>` | ✓ | 删除**文件**（不支持只传路径来删目录） |

### 针对目录的返回（Content Negotiation）

| `Accept` 请求头 | 返回内容 |
|-----------------|----------|
| 包含 `text/html` (浏览器默认) | 渲染美观的 HTML 页面 |
| 包含 `application/json` | JSON 格式的目录树结构 |

### 响应状态码

- `200` : 成功读取或覆盖现有文件
- `201` : 成功新建文件
- `204` : 成功删除文件
- `400` : 错误的请求（比如路径非法、路径穿越、尝试删除目录等）
- `401` : Token 缺失或未匹配
- `404` : 文件或目录不存在

---

## 环境变量

| 变量 | 默认值 | 描述 |
|----------|---------|-------------|
| `ADMIN_TOKEN` | **必填** | 唯一鉴权 Token，用来保护你的写/删权限 |
| `DATA_DIR` | `/data` | 容器内存放文件的根目录（一般不需要改它） |
| `PORT` | `8080` | 服务内部监听端口 |

---

## 快速启动

1. 准备配置文件：
```bash
cp .env.example .env
# 打开并修改你的 .env 文件，设置你喜欢的 ADMIN_TOKEN
```

2. 用 docker compose 启动：
```bash
docker compose up -d
```

默认情况下，宿主机的 `./data` 目录会映射到容器内部的 `/data`。

---

## 使用示例 (curl)

### 上传 (通常配置在 CI 脚本里)

```bash
# PUT：直接传输文件流。（路径必须包含最终文件名，自动帮你建好父目录）
curl -X PUT \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  --data-binary @firmware.bin \
  http://localhost:8080/firmware/v1.0.0/firmware.bin

# POST：通过 multipart form 传输。（路径是一个目录，文件名由表单的 filename 属性决定）
curl -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "file=@app.zip" \
  http://localhost:8080/releases/v1.0.0/
```

### 查看目录 / 下载

```bash
# 浏览器访问，返回美观的 HTML 目录树
curl http://localhost:8080/firmware/

# 脚本访问，加头返回纯 JSON 数据
curl -H "Accept: application/json" http://localhost:8080/firmware/
```

### 删除

```bash
curl -X DELETE \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  http://localhost:8080/firmware/v0.9.0/old.bin
```

---

## Github Actions CI 自动构建

项目自带 `.github/workflows/docker.yml` 工作流，会自动根据提交打 Registry 镜像。如果你 Fork 或复用了这个模板进行自行搭建，你需要配置这些 GitHub Secrets：

| Secret 名称 | 填写说明 | 示例 |
|---|---|---|
| `DOCKERHUB_USERNAME` | 你的 Docker Hub **用户名** | `qiujun` |
| `DOCKERHUB_TOKEN` | Docker Hub 个人 Access Token（在 Account Settings -> Security 中生成）或密码 | `dckr_pat_xxxx` |
| `FILE_SERVER_TOKEN` | 用于展示在后续 CI 给 restfs 发文件时的对应密码。可以任取，和启动服务时的 ADMIN_TOKEN 保持一致即可 | `my_super_secret` |

### 其他项目借助该 CI 传文件的范例：

如果你的某个独立工程需要将 release 的 `app.bin` 直接打包分发给这个 restfs 文件服务器：

```yaml
- name: 上传构建物到 restfs 文件分发服
  run: |
    curl -X PUT \
      -H "Authorization: Bearer ${{ secrets.FILE_SERVER_TOKEN }}" \
      --data-binary @dist/app.bin \
      https://files.example.com/releases/${{ github.ref_name }}/app.bin
```
