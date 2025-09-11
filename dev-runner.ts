#!/usr/bin/env node

import { spawn } from 'child_process'
import chokidar from 'chokidar'

class DevServer {
  private process: any = null
  private restartCount = 0
  private maxRestarts = 10
  private restartDelay = 1000

  constructor() {
    this.startServer()
    this.setupWatcher()
  }

  private startServer() {
    if (this.process) {
      this.process.kill()
    }

    console.log(`[${new Date().toISOString()}] 🚀 启动开发服务器 (重启 #${this.restartCount})`)

    this.process = spawn('bun', ['run', 'server.ts'], {
      stdio: 'inherit',
      env: {
        ...process.env,
        NODE_ENV: 'development'
      }
    })

    this.process.on('exit', (code: number) => {
      if (code !== 0) {
        console.log(`[${new Date().toISOString()}] ❌ 服务器异常退出，代码: ${code}`)
        this.scheduleRestart()
      }
    })

    this.process.on('error', (error: Error) => {
      console.log(`[${new Date().toISOString()}] ❌ 服务器错误:`, error.message)
      this.scheduleRestart()
    })
  }

  private scheduleRestart() {
    if (this.restartCount >= this.maxRestarts) {
      console.log(`[${new Date().toISOString()}] 🛑 达到最大重启次数 (${this.maxRestarts})，停止重启`)
      process.exit(1)
    }

    this.restartCount++
    console.log(`[${new Date().toISOString()}] ⏰ ${this.restartDelay}ms 后重启服务器...`)

    setTimeout(() => {
      this.startServer()
    }, this.restartDelay)
  }

  private setupWatcher() {
    const sourcePaths = ['src/**/*.ts', 'server.ts', 'dev-runner.ts']

    const envPaths = ['.env', '.env.example']

    console.log(`[${new Date().toISOString()}] 🔍 启动源码监听: ${sourcePaths.join(', ')}`)
    console.log(`[${new Date().toISOString()}] 🌍 启动环境变量监听: ${envPaths.join(', ')}`)
    console.log(`[${new Date().toISOString()}] ⚙️ 配置文件变化会自动重载，无需重启`)
    console.log(`[${new Date().toISOString()}] 📝 注意: config.json 变化不会触发重启`)

    const sourceWatcher = chokidar.watch(sourcePaths, {
      ignored: [/node_modules/, 'config.json'],
      persistent: true,
      ignoreInitial: true
    })

    const envWatcher = chokidar.watch(envPaths, {
      persistent: true,
      ignoreInitial: true
    })

    // 源码文件变化处理
    sourceWatcher.on('change', filePath => {
      console.log(`\n[${new Date().toISOString()}] 📝 检测到源码文件变化: ${filePath}`)
      console.log(`[${new Date().toISOString()}] 🔄 自动重启服务器...`)
      this.restartCount = 0
      this.startServer()
    })

    sourceWatcher.on('add', filePath => {
      console.log(`\n[${new Date().toISOString()}] ➕ 检测到新源码文件: ${filePath}`)
      console.log(`[${new Date().toISOString()}] 🔄 自动重启服务器...`)
      this.restartCount = 0
      this.startServer()
    })

    sourceWatcher.on('unlink', filePath => {
      console.log(`\n[${new Date().toISOString()}] 🗑️ 检测到源码文件删除: ${filePath}`)
      console.log(`[${new Date().toISOString()}] 🔄 自动重启服务器...`)
      this.restartCount = 0
      this.startServer()
    })

    // 环境变量文件变化处理
    envWatcher.on('change', filePath => {
      console.log(`\n[${new Date().toISOString()}] 🌍 检测到环境变量文件变化: ${filePath}`)
      console.log(`[${new Date().toISOString()}] 🔄 环境变量变化，自动重启服务器...`)
      this.restartCount = 0
      this.startServer()
    })

    // 优雅关闭
    const gracefulShutdown = () => {
      console.log(`\n[${new Date().toISOString()}] 🛑 正在关闭开发服务器...`)
      if (this.process) {
        this.process.kill()
      }
      sourceWatcher.close()
      envWatcher.close()
      process.exit(0)
    }

    process.on('SIGINT', gracefulShutdown)
    process.on('SIGTERM', gracefulShutdown)
  }
}

// 启动开发服务器
console.log(`[${new Date().toISOString()}] 🎯 Claude API代理开发模式启动`)
console.log(`[${new Date().toISOString()}] 💡 源码文件变化时自动重启服务器`)
console.log(`[${new Date().toISOString()}] ⚙️ 配置文件变化时自动重载配置 (无需重启)`)
console.log(`[${new Date().toISOString()}] 🔧 使用 Ctrl+C 停止服务器\n`)

new DevServer()
