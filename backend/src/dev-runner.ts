#!/usr/bin/env bun
import chokidar from 'chokidar'

// 开发模式运行器
const isDev = process.env.NODE_ENV === 'development' || process.env.NODE_ENV !== 'production'

let serverProcess: any = null

function startServer() {
  console.log('🚀 启动开发服务器...')
  
  serverProcess = Bun.spawn(['bun', 'run', 'src/server.ts'], {
    env: {
      ...process.env,
      NODE_ENV: 'development',
      RUNNER: 'dev-runner'
    },
    stdio: ['inherit', 'inherit', 'inherit']
  })
}

function stopServer() {
  if (serverProcess) {
    console.log('🛑 停止服务器...')
    serverProcess.kill()
    serverProcess = null
  }
}

function restartServer() {
  stopServer()
  setTimeout(startServer, 1000)
}

// 监听文件变化
const watcher = chokidar.watch(['src/**/*.ts'], {
  ignored: [/node_modules/, 'dist'],
  persistent: true,
  ignoreInitial: true
})

watcher.on('change', (filePath) => {
  console.log(`\n📝 检测到文件变化: ${filePath}`)
  console.log('🔄 重启服务器...\n')
  restartServer()
})

// 优雅关闭
process.on('SIGINT', () => {
  console.log('\n👋 收到退出信号...')
  stopServer()
  watcher.close()
  process.exit(0)
})

process.on('SIGTERM', () => {
  stopServer()
  watcher.close()
  process.exit(0)
})

// 启动服务器
startServer()

console.log('\n🔍 开发模式已启动 - 文件变化将自动重启服务器')
console.log('💡 使用 Ctrl+C 停止开发服务器\n')