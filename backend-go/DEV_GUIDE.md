# Go 后端开发指南 - 热重载模式

## 🚀 快速开始

### 1. 安装 Air 热重载工具

Air 项目已迁移至新仓库 `github.com/air-verse/air`（原 `cosmtrek/air`）

```bash
# 推荐方式
make install-air

# 或手动安装
go install github.com/air-verse/air@latest
```

### 2. 启动热重载开发模式

```bash
# 启动开发模式
make dev

# 输出示例：
# 🚀 启动开发模式 (热重载开启)
# 📝 监听文件变化: *.go, *.yaml, *.toml, *.env
# 🔄 修改代码后将自动重启...
```

## 📁 热重载配置

### 监听的文件类型
- `*.go` - Go 源代码
- `*.yaml`, `*.yml` - YAML 配置文件
- `*.toml` - TOML 配置文件
- `*.env` - 环境变量文件
- `*.html`, `*.tpl`, `*.tmpl` - 模板文件

### 忽略的目录
- `tmp/` - Air 临时编译目录
- `vendor/` - Go 依赖目录
- `frontend/` - 前端源码（不影响后端）
- `dist/` - 构建输出目录
- `.git/`, `.github/` - Git 相关
- `.vscode/`, `.idea/` - IDE 配置

### 性能优化设置
- **编译延迟**: 1000ms（避免保存过程中频繁编译）
- **错误处理**: 编译错误时保持旧版本运行
- **信号处理**: 优雅关闭，500ms 延迟
- **清屏设置**: 每次重编译时自动清屏

## 🎯 开发流程

### 典型开发场景

1. **修改业务逻辑**
   ```bash
   # 编辑 handlers/proxy.go
   # 保存文件 → Air 检测到变化 → 1秒后自动重编译 → 重启服务
   ```

2. **更新配置文件**
   ```bash
   # 编辑 .env
   # 保存文件 → Air 重新加载配置 → 服务自动重启
   ```

3. **处理编译错误**
   ```bash
   # 代码有语法错误
   # Air 显示错误信息 → 保持旧版本运行
   # 修复错误并保存 → 自动重新编译 → 恢复正常
   ```

## 🛠️ Make 命令参考

| 命令 | 说明 | 使用场景 |
|------|------|---------|
| `make dev` | 启动热重载开发模式 | 日常开发主要命令 |
| `make install-air` | 安装 Air 工具 | 首次设置或更新 Air |
| `make run` | 直接运行（无热重载） | 快速测试 |
| `make build` | 构建生产版本 | 部署准备 |
| `make build-local` | 构建本地版本 | 本地测试 |
| `make test` | 运行测试 | 功能验证 |
| `make fmt` | 格式化代码 | 代码规范化 |
| `make clean` | 清理临时文件 | 清理环境 |

## 🔧 Air 高级配置

### 自定义 .air.toml

```toml
# 添加预编译命令
[build.pre_cmd]
  enable = true
  cmds = [
    "echo '开始编译...'",
    "go mod tidy"
  ]

# 添加后编译命令
[build.post_cmd]
  enable = true
  cmds = [
    "echo '编译完成！'"
  ]

# 自定义运行参数
[build]
  # 添加构建标签
  cmd = "go build -tags dev -o ./tmp/main ."

  # 传递运行时参数
  args_bin = ["--debug", "--verbose"]
```

### 环境变量

```bash
# 开发模式专用环境变量
ENV=development
LOG_LEVEL=debug
ENABLE_REQUEST_LOGS=true
ENABLE_RESPONSE_LOGS=true
```

## 🐛 问题排查

### Air 命令未找到
```bash
# 检查安装
which air

# 添加到 PATH
export PATH=$PATH:$(go env GOPATH)/bin

# 或添加到 shell 配置
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.zshrc
source ~/.zshrc
```

### 热重载不触发
```bash
# 检查 Air 进程
ps aux | grep air

# 查看 Air 日志
tail -f build-errors.log

# 清理并重启
make clean && make dev
```

### 端口占用
```bash
# 查找占用端口的进程
lsof -i :3000

# 终止进程
kill -9 <PID>
```

### 文件权限问题
```bash
# 修复权限
chmod -R 755 .
chmod 644 .air.toml
```

## 📊 性能对比

| 操作 | 无热重载 | 有热重载 | 提升 |
|------|---------|---------|------|
| 修改代码后重启 | 手动 10-15秒 | 自动 1-2秒 | **10倍** |
| 处理编译错误 | 中断→修复→重启 | 保持运行→修复→自动恢复 | **无中断** |
| 配置更新 | 停止→修改→启动 | 修改→自动重启 | **3倍** |
| 开发效率 | 低 | 高 | **显著提升** |

## 💡 最佳实践

1. **保持 Air 运行**: 开发期间始终使用 `make dev`
2. **合理设置延迟**: 1秒延迟平衡了响应速度和性能
3. **利用彩色输出**: 不同颜色快速区分日志类型
4. **定期清理**: 使用 `make clean` 清理临时文件
5. **版本控制**: `.air.toml` 应该加入版本控制

## 🔗 相关资源

- [Air 官方文档](https://github.com/air-verse/air)
- [Air 配置示例](https://github.com/air-verse/air/blob/master/air_example.toml)
- [Gin 开发模式文档](https://gin-gonic.com/docs/quickstart/)

---

**提示**: 如果遇到任何问题，请先运行 `make clean && make install-air` 重置环境。