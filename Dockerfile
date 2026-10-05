# VideoDelite account server - Linux/Docker image (Debian deployment).
#
# Build:  docker build -t videodelite-server .
# Run:    docker compose up -d     (recommended, includes SQL Server)
#
# 国内网络拉不动 Docker Hub 时（二选一）：
#   a) 配置 daemon 镜像加速（见 docs/DEPLOY-DEBIAN.md 第一节），命令不变；
#   b) 构建时指定镜像前缀：
#      docker build \
#        --build-arg GO_IMAGE=docker.m.daocloud.io/library/golang:1.27-alpine \
#        --build-arg RUN_IMAGE=docker.m.daocloud.io/library/debian:bookworm-slim \
#        -t videodelite-server .
ARG GO_IMAGE=golang:1.27-alpine
ARG RUN_IMAGE=debian:bookworm-slim

FROM ${GO_IMAGE} AS build
WORKDIR /src
# 国内构建必备：Go 模块代理（海外环境同样可用）
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /videoserver ./cmd/videoserver

FROM ${RUN_IMAGE}
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /videoserver /usr/local/bin/videoserver
# /data holds jwt.secret (named volume; docker handles permissions)
ENV VIDEODELITE_LISTEN="0.0.0.0:8800" \
    VIDEODELITE_JWT_SECRET_FILE="/data/jwt.secret"
EXPOSE 8800
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/videoserver"]
