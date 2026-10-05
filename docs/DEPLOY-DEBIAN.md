# VideoDelite 服务端 Debian (Docker) 部署指南

> **当前生产拓扑（2026-10-04 定稿）**：服务容器 8800（内部）+ nginx-proxy 28443→443
> （HTTPS，Let's Encrypt 证书 acme.sh DNS-01 自动续期）。
> **客户端/管理台入口：`https://videodelite1.898280.xyz:28443` / `https://videodelite.898280.xyz:28443/admin`**
> 80/443/8080/8443 被 ISP 封禁，勿用；内网主机需 hosts 指向 192.168.100.101。

服务端是纯 Go 静态二进制，官方支持 Linux。推荐 Docker Compose 一键部署
（自带 SQL Server 2022 容器，无需在宿主机装数据库）。

## 一、前置要求

```bash
# Debian 11/12
sudo apt update && sudo apt install -y docker.io docker-compose-v2
sudo systemctl enable --now docker
```

### 国内网络：配置 Docker 镜像加速（国内服务器必做）

Docker Hub 国内直连经常超时。配置加速器后拉取 `golang` / `debian` 等
官方镜像会自动走国内节点：

```bash
sudo tee /etc/docker/daemon.json <<'EOF'
{
  "registry-mirrors": [
    "https://docker.m.daocloud.io",
    "https://docker.1ms.run",
    "https://hub.rat.dev"
  ]
}
EOF
sudo systemctl restart docker
```

> 加速器地址时效性较强，若上面几个不可用，可搜索"Docker 镜像加速"获取
> 当前可用节点，或改用方案 B：构建时直接指定镜像前缀（见 Dockerfile
> 头部注释），compose 里 db 镜像同理改为
> `docker.m.daocloud.io/mcr.microsoft.com/mssql/server:2022-latest`。
> mcr.microsoft.com（SQL Server 镜像）国内一般可直连，无需加速。

## 二、上传代码并启动

把整个 VideoDelite 项目文件夹上传到服务器（或 `git clone`），然后：

```bash
cd VideoDelite
sudo docker compose up -d --build
```

首次启动会：
1. 拉取 SQL Server 2022 镜像并初始化（约 1-2 分钟）
2. 自动创建 `videodelite` 数据库和全部表
3. 服务端监听 8800 端口

验证：

```bash
curl http://127.0.0.1:8800/api/v1/version
# 管理后台：
# http://<服务器IP>:8800/admin
```

## 三、修改配置

所有配置通过 `docker-compose.yml` 的 environment 设置（环境变量优先于
config 文件）：

| 环境变量 | 说明 |
|---|---|
| `VIDEODELITE_DB_CONN` | SQL Server 连接串（默认连 compose 里的 db 容器） |
| `VIDEODELITE_ADMIN_KEY` | 管理后台密钥（**上线前必须改**） |
| `VIDEODELITE_MAIL_PROVIDER` | `resend` / `smtp` / 空（dev-echo） |
| `VIDEODELITE_MAIL_FROM` | 发件地址 `VideoDelite <noreply@videodelite.898280.xyz>` |
| `VIDEODELITE_MAIL_APIKEY` | Resend API Key |
| `VIDEODELITE_INVITE_REQUIRED` | `1` = 注册需要邀请码 |

修改后 `sudo docker compose up -d` 重建生效。

数据持久化：`mssql-data` 卷（数据库）、`server-data` 卷（JWT 密钥），
删除容器不丢数据。

## 四、防火墙

```bash
sudo ufw allow 8800/tcp    # 或 iptables / 云厂商安全组放行 8800
```

## 五、客户端连接

VideoDelite 桌面程序 → 设置 → 账号 → API 服务地址：

```
http://<服务器IP>:8800
```

正式对外建议套 HTTPS（Caddy 示例）：

```
# Caddyfile
api.videodelite.898280.xyz {
    reverse_proxy 127.0.0.1:8800
}
```

客户端填 `https://api.videodelite.898280.xyz`。

## 六、常用运维命令

```bash
sudo docker compose logs -f server     # 看服务端日志
sudo docker compose restart server     # 重启
sudo docker compose down               # 停止（保留数据）
sudo docker compose down -v            # 停止并删除数据（慎用）
```

## 七、从 Windows 测试环境迁移说明

- Debian 上是新空库，之前 Windows 测试库的账号不会自动迁移（测试数据无需迁移）
- 域名发信配置（Resend DNS 记录）与服务器无关，无需改动
- GUI 客户端只需改"API 服务地址"指向新服务器
