# go-wecomchan

一个带 Web 管理界面的企业微信消息服务。

第一次使用，直接按下面 5 步做就够了。

## 1. 启动 `sola97/wecomchan-next + redis + nginx`

新建一个 `docker-compose.yml`，内容直接用下面这份：

```yaml
services:
  wecomchan:
    image: sola97/wecomchan-next:latest
    container_name: wecomchan-next
    restart: always
    environment:
      WEB_PASSWORD: "<修改成你的密码>"
      BOT_CONFIG_PATH: /root/data/bot-config.json
    volumes:
      - ./data:/root/data
    depends_on:
      - redis
    entrypoint:
      - /bin/sh
      - -c
    command: |
      mkdir -p /root/data
      if [ ! -f /root/data/bot-config.json ]; then
        cat >/root/data/bot-config.json <<EOF
      {
        "redis": {
          "enabled": true,
          "addr": "redis:6379",
          "password": "<修改成你的密码>"
        },
        "configs": []
      }
      EOF
      fi
      exec ./wecomchan
    logging:
      driver: json-file
      options:
        max-size: 10m
        max-file: "1"

  redis:
    image: bitnami/redis:latest
    container_name: wecomchan-next-redis
    restart: always
    environment:
      REDIS_DISABLE_COMMANDS: FLUSHDB,FLUSHALL
      REDIS_PASSWORD: "<修改成你的密码>"
    volumes:
      - redis_data:/bitnami/redis/data
    logging:
      driver: json-file
      options:
        max-size: 10m
        max-file: "1"

  nginx:
    image: sola97/nginx:latest
    container_name: wecomchan-next-nginx
    restart: always
    depends_on:
      - wecomchan
    ports:
      - "51080:80"
    entrypoint:
      - /bin/sh
      - -c
    command: |
      cat >/etc/nginx/conf.d/default.conf <<EOF
      server {
          listen 80;
          server_name _;

          location / {
              proxy_pass http://wecomchan:8080;
              proxy_http_version 1.1;
              proxy_set_header Host \$$host;
              proxy_set_header X-Real-IP \$$remote_addr;
              proxy_set_header X-Forwarded-For \$$proxy_add_x_forwarded_for;
              proxy_set_header X-Forwarded-Proto \$$scheme;
          }
      }
      EOF
      exec nginx -g 'daemon off;'
    logging:
      driver: json-file
      options:
        max-size: 30m
        max-file: "1"

volumes:
  redis_data:
```

把上面两个 `<修改成你的密码>` 都改掉。

启动：

```bash
docker compose up -d
```

启动后访问：

- `http://<服务器IP>:51080/admin/`

### 登录页

<img src="docs/images/readme-admin-login.png" alt="管理界面登录页" width="960" />

## 2. 进入后台，新建机器人配置

登录后台后：

1. 点击“新建配置”
2. 填入机器人配置
3. 点击“保存配置”

保存后，左侧列表会出现这一个机器人。

### 单机器人配置页

<img src="docs/images/readme-admin-config.png" alt="单机器人配置页面" width="960" />

## 3. 先去企业微信后台，打开“接收消息服务器配置”

先进入企业微信应用后台的“接收消息服务器配置”页面。

这一步先准备好：

- `Token`
- `EncodingAESKey`

### 接收消息服务器配置

<img src="docs/images/readme-admin-callback.png" alt="接收消息服务器配置" width="960" />

## 4. 回到网页填写，再回企业微信保存

回到网页里的“接收消息服务器配置”区块：

- 把企业微信页面里的 `Token` 填进网页
- 把企业微信页面里的 `EncodingAESKey` 填进网页
- 点击保存
- `URL` 直接用网页展示出来的地址

然后再回到企业微信后台，把下面这 3 个值填进去并保存：

- `URL`
- `Token`
- `EncodingAESKey`

建议直接用最终域名访问管理界面，再去复制这组配置。

## 5. 企业微信保存成功后，再设置可信 IP 和接口测试

企业微信里的“接收消息服务器配置”保存成功后：

1. 设置可信 IP
2. 回到网页里的“接口测试”页面发一条测试消息

接口模板、参数说明、执行按钮、执行结果，网页端都已经有了，README 不再重复写。

### 接口测试页

<img src="docs/images/readme-admin-tools.png" alt="接口测试页面" width="960" />

## 说明

- 配置会保存到 `./data/bot-config.json`
- 如果只是第一次上线，优先走单机器人流程
- 接口说明、回调地址、执行结果都以网页端为准
