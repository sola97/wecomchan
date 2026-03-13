# go-wecomchan

一个带 Web 管理界面的企业微信消息服务。

第一次使用，直接按下面 5 步做就够了。

## 1. 启动 `sola97/wecomchan-next + redis + nginx`

先准备配置文件：

```bash
cp bot-config.empty.example.json bot-config.json
cp nginx.conf.example nginx.conf
mkdir -p conf.d
cp conf.d/wecomchan.conf.example conf.d/wecomchan.conf
```

然后只改这几个地方：

- `docker-compose.new.yaml` 里的 `WEB_PASSWORD`
- `docker-compose.new.yaml` 里的 `REDIS_PASSWORD`
- `conf.d/wecomchan.conf` 里的 `server_name`

启动：

```bash
docker compose -f docker-compose.new.yaml up -d
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

## 3. 进入“接收消息服务器配置”，填好后保存

在机器人编辑页的“接收消息服务器配置”里：

- `URL` 直接用页面展示出来的地址
- 填入 `Token`
- 填入 `EncodingAESKey`
- 点击保存

这里不要先记接口细节，直接以页面展示为准。

### 接收消息服务器配置

<img src="docs/images/readme-admin-callback.png" alt="接收消息服务器配置" width="960" />

## 4. 去企业微信后台设置

在企业微信应用里完成两件事：

1. 设置可信 IP
2. 设置接收消息服务器配置

把后台页面里看到的：

- `URL`
- `Token`
- `EncodingAESKey`

原样填到企业微信里即可。

建议直接用最终域名访问管理界面，再去复制这组配置。

## 5. 用“接口测试”页面自测

机器人保存完成后，直接进入“接口测试”页面发一条测试消息。

接口模板、参数说明、执行按钮、执行结果，网页端都已经有了，README 不再重复写。

### 接口测试页

<img src="docs/images/readme-admin-tools.png" alt="接口测试页面" width="960" />

## 相关文件

- `docker-compose.new.yaml`
- `bot-config.empty.example.json`
- `bot-config.single.example.json`
- `bot-config.example.json`
- `nginx.conf.example`
- `conf.d/wecomchan.conf.example`

## 说明

- 配置会保存到挂载的 `bot-config.json`
- 如果只是第一次上线，优先走单机器人流程
- 接口说明、回调地址、执行结果都以网页端为准
