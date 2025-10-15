# --- 阶段 1: 准备 Bun 运行时 ---
FROM oven/bun:alpine AS bun-runtime

# --- 阶段 2: 构建阶段 (Go + Bun) ---
FROM golang:1.22-alpine AS builder

WORKDIR /src

# 安装必要的构建工具和 bun 依赖（libstdc++ libgcc 是 bun:alpine 运行所需）
RUN apk add --no-cache git make libstdc++ libgcc

# 从 bun-runtime 复制 bun 和 bunx 到 Go 镜像
COPY --from=bun-runtime /usr/local/bin/bun /usr/local/bin/bun
COPY --from=bun-runtime /usr/local/bin/bunx /usr/local/bin/bunx

# 将 bun 添加到 PATH
ENV PATH="/usr/local/bin:${PATH}"

# 复制项目必要文件（.dockerignore 会排除不需要的文件）
COPY Makefile VERSION tsconfig.json ./
COPY frontend/ ./frontend/
COPY backend-go/ ./backend-go/

# 使用 bun 安装前端依赖（比 npm 快 10-100 倍）
RUN cd frontend && bun install

# 使用 Makefile 构建整个项目（前端 + 后端）
RUN make build-current

# --- 阶段 3: 运行时 ---
FROM alpine:latest AS runtime

WORKDIR /app

# 安装运行时依赖
RUN apk --no-cache add ca-certificates tzdata

# 从构建阶段复制 Go 二进制文件（已内嵌前端资源）
COPY --from=builder /src/backend-go/dist/claude-proxy /app/claude-proxy

# 创建配置目录和日志目录
RUN mkdir -p /app/.config/backups /app/logs

# 设置时区（可选）
ENV TZ=Asia/Shanghai

# 暴露端口
EXPOSE 3000

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/health || exit 1

# 启动命令
CMD ["/app/claude-proxy"]
