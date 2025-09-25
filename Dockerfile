# --- 阶段 1: 项目构建 ---
FROM oven/bun:latest AS builder

WORKDIR /src

# 复制项目根目录配置文件
COPY package.json ./
COPY tsconfig.json ./

# 复制backend必要文件
COPY backend/src ./backend/src
COPY backend/package.json ./backend/package.json
COPY backend/tsconfig.json ./backend/tsconfig.json

# 复制frontend必要文件
COPY frontend/src ./frontend/src
COPY frontend/index.html ./frontend/index.html
COPY frontend/package.json ./frontend/package.json
COPY frontend/postcss.config.js ./frontend/postcss.config.js
COPY frontend/tailwind.config.js ./frontend/tailwind.config.js
COPY frontend/tsconfig.json ./frontend/tsconfig.json
COPY frontend/vite.config.ts ./frontend/vite.config.ts

# 安装所有依赖并构建整个项目
RUN bun install
RUN bun run build

# --- 阶段 2: 前端运行时 ---
FROM nginx:alpine AS frontend-runtime

WORKDIR /app/frontend

# 复制前端构建产物到nginx目录
COPY --from=builder /src/frontend/dist /usr/share/nginx/html

# 创建nginx配置文件
RUN cat > /etc/nginx/conf.d/default.conf << 'EOF'
server {
    listen 5173;
    server_name localhost;
    
    # 前端静态文件
    location / {
        root /usr/share/nginx/html;
        index index.html index.htm;
        try_files $uri $uri/ /index.html;
    }
    
    # 代理API请求到后端
    location /api {
        proxy_pass http://claude-proxy-backend:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # 代理v1 API请求到后端
    location /v1 {
        proxy_pass http://claude-proxy-backend:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

# 暴露前端端口
EXPOSE 5173

# 启动nginx
CMD ["nginx", "-g", "daemon off;"]

# --- 阶段 3: 后端运行时 ---
FROM oven/bun:latest AS backend-runtime

WORKDIR /app

# 从构建阶段复制后端代码和依赖
COPY --from=builder --chown=bun:bun /src/backend ./

# 创建配置目录和日志目录
RUN mkdir -p /app/.config/backups /app/logs && chown bun:bun /app/.config /app/.config/backups /app/logs

# 暴露后端端口
EXPOSE 3000

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD bun run -e 'await fetch("http://localhost:3000/health").then(r => r.ok ? process.exit(0) : process.exit(1))' || exit 1

# 切换到非root用户
USER bun

# 设置后端端口环境变量
ENV PORT=3000

# 启动命令 - 直接运行编译后的文件
CMD ["bun", "run", "dist/server.js"]
