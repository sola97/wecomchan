# go-wecomchan

一个可通过企业微信应用发送消息的轻量服务，支持：

- `GET /wecomchan` 发送文本
- `POST /wecomchan` 发送文本、Markdown、图片，以及图文双发
- `GET /<route>/callback` 企业微信回调地址校验
- `GET /admin` 登录后进入 Web 管理界面，内置接口模板、执行按钮和结果回显

## 新增能力

- 增加了 React + TailwindCSS 编写的 Web 管理界面
- 管理界面通过环境变量 `WEB_PASSWORD` 控制登录
- 管理界面提供机器人配置、Redis 配置、接口测试、7 天发送日志四个页面
- 管理界面提供 GET、POST、图片、图文双发四类调试工具
- 管理界面提供企业微信消息校验接口的地址和参数说明
- 大于 2MB 的测试图片会在前端自动尝试 JPEG 压缩
- 机器人和 Redis 配置支持保存到本地 JSON 文件
- Docker 多阶段构建会自动编译前端并复制 `frontend/dist`

## 环境变量

| 名称 | 描述 | 默认值 |
| --- | --- | --- |
| `SENDKEY` | 公开发送接口的校验密钥 | `set_a_sendkey` |
| `WEB_PASSWORD` | 管理界面登录密码 | 空 |
| `LISTEN_ADDR` | 服务监听地址 | `:8080` |
| `FRONTEND_DIST_DIR` | 前端静态文件目录 | `./frontend/dist` |
| `BOT_CONFIG_PATH` | 多机器人 JSON 配置文件路径 | `./data/bot-config.json` |
| `MESSAGE_LOG_PATH` | 发送日志 JSONL 文件路径 | `./data/message-logs.jsonl` |
| `WECOM_CID` | 企业微信公司 ID | `企业微信公司ID` |
| `WECOM_SECRET` | 企业微信应用 Secret | `企业微信应用Secret` |
| `WECOM_AID` | 企业微信应用 ID | `企业微信应用ID` |
| `WECOM_TOUID` | 消息接收对象 | `@all` |
| `REDIS_STAT` | 是否启用 Redis 缓存 token，`ON`/`OFF` | `OFF` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `REDIS_PASSWORD` | Redis 密码 | 空 |
| `WECOM_TOKEN` | 企业微信回调 Token | `企业微信回调Token` |
| `WECOM_AES_KEY` | 企业微信回调 AesKey | `企业微信回调AesKey` |

`WEB_PASSWORD` 未配置时，`/admin` 页面仍可访问，但不会允许登录。

## 配置文件

仓库默认只提交示例文件，不提交真实密钥：

- `bot-config.example.json`：多机器人配置模板
- `docker-compose.new.yaml`：`wecomchan-next` 的容器部署模板

首次使用建议：

```bash
cp bot-config.example.json bot-config.json
```

然后按你的真实企业微信参数修改 `bot-config.json`，再挂载到容器内。

## 本地启动

### 1. 构建前端

```bash
cd frontend
npm install
npm run build
```

### 2. 启动后端

```bash
export SENDKEY=set_a_sendkey
export WEB_PASSWORD=change_me
export WECOM_CID=企业微信公司ID
export WECOM_SECRET=企业微信应用Secret
export WECOM_AID=企业微信应用ID

go run .
```

启动后可访问：

- 管理界面：`http://localhost:8080/admin`
- 服务健康页：`http://localhost:8080/`

## Docker 构建

### 构建镜像

```bash
docker build -t go-wecomchan .
```

### 启动容器

```bash
docker run -dit \
  -p 8080:8080 \
  -e SENDKEY=set_a_sendkey \
  -e WEB_PASSWORD=change_me \
  -e WECOM_CID=企业微信公司ID \
  -e WECOM_SECRET=企业微信应用Secret \
  -e WECOM_AID=企业微信应用ID \
  go-wecomchan
```

## docker-compose

项目内有两个 compose 模板：

- `docker-compose.yml`：本地源码构建示例
- `docker-compose.new.yaml`：`sola97/wecomchan-next:latest` 镜像部署示例

`docker-compose.new.yaml` 默认挂载本地配置文件：

```yaml
volumes:
  - ./bot-config.json:/root/data/bot-config.json
```

使用前请先复制并编辑：

```bash
cp bot-config.example.json bot-config.json
```

示例内容已经包含：

- `WEB_PASSWORD`
- `redis`
- `BOT_CONFIG_PATH`
- JSON 配置文件挂载

执行：

```bash
docker compose -f docker-compose.new.yaml up -d
```

## 接口调用示例

### 1. GET 发送文本

```bash
curl --location --request GET \
  'http://localhost:8080/wecomchan?sendkey=set_a_sendkey&msg=hello&msg_type=text'
```

### 2. POST 发送文本 / Markdown

```bash
curl --location --request POST \
  'http://localhost:8080/wecomchan' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "sendkey": "set_a_sendkey",
    "msg": "## hello from markdown",
    "msg_type": "markdown"
  }'
```

### 3. POST 发送图片

```bash
curl --location --request POST \
  'http://localhost:8080/wecomchan?sendkey=set_a_sendkey&msg_type=image' \
  --form 'media=@"./test.png"'
```

### 4. POST 图文双发

```bash
curl --location --request POST \
  'http://localhost:8080/wecomchan' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "sendkey": "set_a_sendkey",
    "msg": "这是一条文字消息",
    "msg_type": "text",
    "image": "<BASE64_IMAGE_DATA>",
    "filename": "demo.png"
  }'
```

说明：

- 图文双发直接复用原来的 `POST /wecomchan`
- `image` 传图片的 base64 字符串
- 为兼容旧调用，后端仍兼容 `image_base62` 和 `image_data` 别名
- 后端会先发送一条 `text` 消息，再发送一条 `image` 消息
- 图片大小仍受企业微信 2MB 限制

## Web 管理界面

访问：

```text
http://localhost:8080/admin
```

登录后可以直接使用：

- 机器人配置列表与右侧编辑表单联动
- Redis 全局配置页面
- GET 文本发送模板
- POST 文本 / Markdown 模板
- 图片发送模板
- 图文双发模板
- 最近 7 天发送日志
- 企业微信回调校验接口说明

## 回调接口说明

### GET `/<route>/callback`

用于企业微信回调 URL 校验，查询参数：

- `msg_signature`
- `timestamp`
- `nonce`
- `echostr`

### POST `/<route>/callback`

当前项目保留该入口，便于后续扩展企业微信消息回调处理。常见参数：

- `msg_signature`
- `timestamp`
- `nonce`
- 请求体为企业微信 XML 加密消息
