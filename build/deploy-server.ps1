# VideoDelite account server deployment packaging (Phase 11).
# Produces build/dist/VideoDelite-Server-deploy.zip - copy to the target
# machine (e.g. 192.168.100.101), unzip, run videoserver.exe.
#
# Usage: powershell -ExecutionPolicy Bypass -File build\deploy-server.ps1
$ErrorActionPreference = "Stop"

Write-Host "== build videoserver ==" -ForegroundColor Cyan
go build -o build\dist\videoserver.exe ./cmd/videoserver
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Write-Host "== staging deploy folder ==" -ForegroundColor Cyan
$stage = "build\dist\VideoDelite-Server"
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item "build\dist\videoserver.exe" $stage

# Server config for the TARGET machine: SQL Server and mail live there.
# DB points to localhost because the server runs ON the SQL Server host.
@'
{
  "listen": "0.0.0.0:8800",
  "db": {
    "connection": "sqlserver://sa:<SA_PASSWORD>@127.0.0.1:1433?database=videodelite&encrypt=disable",
    "autoCreateDatabase": true
  },
  "mail": {
    "provider": "resend",
    "from": "VideoDelite <noreply@videodelite.898280.xyz>",
    "apiKey": "<RESEND_API_KEY>"
  },
  "inviteRequired": false,
  "devEchoMail": false,
  "adminKey": "vd-admin-test",
  "jwtSecretFile": "jwt.secret",
  "accessTtlMinutes": 15,
  "refreshTtlDays": 30
}
'@ | Out-File "$stage\config.json" -Encoding utf8

# Deployment readme
@'
VideoDelite 账号服务端 - 部署说明
====================================

目标机器要求
  - Windows 10/11 x64
  - 本机已安装 SQL Server（1433 端口，账号 sa）
  - 能访问外网（发验证码走 Resend API）

部署步骤
  1. 将整个文件夹复制到目标机器任意目录（如 C:\VideoDelite）
  2. 双击 videoserver.exe（或在 cmd 中运行以便查看日志）
     - 首次启动会自动创建 videodelite 数据库和表
  3. 验证：浏览器打开 http://<本机IP>:8800/admin
     - 管理密钥见 config.json 的 adminKey（测试值 vd-admin-test，正式部署请修改）
  4. 客户端（VideoDelite 桌面程序）设置 → 账号 → API 服务地址
     填 http://<本机IP>:8800 即可注册登录

防火墙
  如客户端无法连接，放行 8800 端口：
    netsh advfirewall firewall add rule name="VideoDelite" dir=in action=allow protocol=TCP localport=8800

注册为 Windows 服务（可选，开机自启）
  sc create VideoDeliteServer binPath= "C:\VideoDelite\videoserver.exe" start= auto
  sc start VideoDeliteServer

安全清单（正式对外前）
  - 修改 config.json 的 adminKey 为强随机值
  - SQL Server 启用加密（encrypt=true + 证书）
  - 限制 sa 使用，创建专用低权限账号
  - 配置 HTTPS 反向代理（如 Caddy/Nginx）
'@ | Out-File "$stage\部署说明.txt" -Encoding utf8

Write-Host "== zip ==" -ForegroundColor Cyan
$zip = "build\dist\VideoDelite-Server-deploy.zip"
if (Test-Path $zip) { Remove-Item $zip }
Compress-Archive -Path $stage -DestinationPath $zip

Write-Host "== done ==" -ForegroundColor Green
Get-Item $zip | Format-Table Name, Length
