#!/usr/bin/env node
import { configManagerCLI, UpstreamConfig } from './src/config'
import { redisCache } from './src/redis'

// 配置命令执行完成后自动退出
process.on('exit', async () => {
    await redisCache.disconnect()
})

function showHelp() {
    console.log(`
Claude API代理服务器配置工具

使用方法:
  bun run config <command> [options]

命令:
  show                    显示当前配置
  add <name> <type> <url> 添加上游配置
  remove <index>          删除上游配置
  update <index> [options] 更新上游配置
  use <index|name>        切换到指定上游（支持索引或名称）
  key <index> <action>    管理API密钥
  balance <strategy>      设置负载均衡策略
  help                    显示帮助信息

参数:
  name                    上游名称
  type                    服务类型 (gemini, openai, oainew, claude, custom)
  url                     上游基础URL
  index                   上游索引
  action                  密钥操作 (add, remove, list)
  strategy                负载均衡策略 (round-robin, random, failover)

示例:
  bun run config show                              # 显示当前配置
  bun run config add MyGemini gemini https://generativelanguage.googleapis.com/v1beta
  bun run config add MyOpenAI openai https://api.openai.com/v1
  bun run config use 0                             # 切换到第一个上游
  bun run config use nonocode                     # 切换到名称为 nonocode 的上游
  bun run config key 0 add sk-1234567890abcdef      # 添加API密钥
  bun run config key 0 list                        # 列出API密钥
  bun run config update 0 --name "NewName"         # 更新上游名称
  bun run config update 0 --description "基于gpt-4o，响应速度快" # 更新备注
  bun run config balance round-robin              # 设置负载均衡
  bun run config remove 0                          # 删除上游

支持的默认URL:
  gemini: https://generativelanguage.googleapis.com/v1beta
  openai: https://api.openai.com/v1
  oainew: https://api.openai.com/v1
`)
}

function parseArgs(args: string[]): { [key: string]: string } {
    const parsed: { [key: string]: string } = {}
    for (let i = 0; i < args.length; i++) {
        if (args[i].startsWith('--')) {
            const key = args[i].slice(2)
            const value = args[i + 1] && !args[i + 1].startsWith('--') ? args[i + 1] : 'true'
            parsed[key] = value
            if (value !== 'true') i++
        }
    }
    return parsed
}

function addUpstream(name: string, type: string, url: string, extraArgs?: string[]) {
    if (!['gemini', 'openai', 'oainew', 'claude', 'custom'].includes(type)) {
        console.error('错误: 不支持的类型，请使用 gemini, openai, oainew, claude 或 custom')
        return
    }

    const upstream: UpstreamConfig = {
        name,
        serviceType: type as any,
        baseUrl: url,
        apiKeys: []
    }

    // 解析额外参数
    if (extraArgs && extraArgs.length > 0) {
        const flags = parseArgs(extraArgs)
        if (flags.description) {
            upstream.description = flags.description
        }
    }

    configManagerCLI.addUpstream(upstream)
}

function updateUpstream(index: string, args: string[]) {
    const upstreamIndex = parseInt(index)
    const flags = parseArgs(args)

    if (isNaN(upstreamIndex)) {
        console.error('错误: 无效的上游索引')
        return
    }

    const update: Partial<UpstreamConfig> = {}
    if (flags.name) update.name = flags.name
    if (flags.url) update.baseUrl = flags.url
    if (flags.type) update.serviceType = flags.type as any
    if (flags.description) update.description = flags.description

    if (Object.keys(update).length === 0) {
        console.error('错误: 请指定要更新的字段 (--name, --url, --type, --description)')
        return
    }

    configManagerCLI.updateUpstream(upstreamIndex, update)
}

function manageKeys(index: string, action: string, args: string[]) {
    const upstreamIndex = parseInt(index)

    if (isNaN(upstreamIndex)) {
        console.error('错误: 无效的上游索引')
        return
    }

    switch (action) {
        case 'add':
            const apiKey = args[0]
            if (!apiKey) {
                console.error('错误: 请提供API密钥')
                return
            }
            configManagerCLI.addApiKey(upstreamIndex, apiKey)
            break

        case 'remove':
            const keyToRemove = args[0]
            if (!keyToRemove) {
                console.error('错误: 请提供要删除的API密钥')
                return
            }
            configManagerCLI.removeApiKey(upstreamIndex, keyToRemove)
            break

        case 'list':
            const config = configManagerCLI.getConfig()
            const upstream = config.upstream[upstreamIndex]
            if (upstream) {
                console.log(`上游 [${upstreamIndex}] 的API密钥:`)
                if (upstream.apiKeys.length === 0) {
                    console.log('  没有API密钥')
                } else {
                    upstream.apiKeys.forEach((key, i) => {
                        console.log(`  [${i}] ${key}`)
                    })
                }
            } else {
                console.error('错误: 上游不存在')
            }
            break

        default:
            console.error('错误: 不支持的密钥操作，请使用 add, remove 或 list')
    }
}

function main() {
    const args = process.argv.slice(2)

    if (args.length === 0 || args[0] === 'help') {
        showHelp()
        return
    }

    const command = args[0]

    switch (command) {
        case 'show':
            configManagerCLI.showConfig()
            break

        case 'add':
            if (args.length < 4) {
                console.error('错误: add 命令需要 name, type 和 url 参数')
                console.log('示例: bun run config add MyGemini gemini https://generativelanguage.googleapis.com/v1beta')
                console.log('      bun run config add MyOpenAI openai https://api.openai.com/v1 --description "官方OpenAI接口"')
                return
            }
            addUpstream(args[1], args[2], args[3], args.slice(4))
            break

        case 'remove':
            if (args.length < 2) {
                console.error('错误: remove 命令需要 index 参数')
                return
            }
            configManagerCLI.removeUpstream(parseInt(args[1]))
            break

        case 'update':
            if (args.length < 2) {
                console.error('错误: update 命令需要 index 参数')
                return
            }
            updateUpstream(args[1], args.slice(2))
            break

        case 'use':
            if (args.length < 2) {
                console.error('错误: use 命令需要 index 或 name 参数')
                return
            }
            const indexOrName = args[1]
            try {
                // 尝试解析为数字，如果失败则作为名称处理
                const parsedIndex = parseInt(indexOrName)
                if (!isNaN(parsedIndex)) {
                    configManagerCLI.setUpstream(parsedIndex)
                } else {
                    configManagerCLI.setUpstream(indexOrName)
                }
            } catch (error) {
                console.error(`错误: ${error instanceof Error ? error.message : error}`)
            }
            break

        case 'key':
            if (args.length < 3) {
                console.error('错误: key 命令需要 index 和 action 参数')
                return
            }
            manageKeys(args[1], args[2], args.slice(3))
            break

        case 'balance':
            if (args.length < 2) {
                console.error('错误: balance 命令需要 strategy 参数')
                return
            }
            const strategy = args[1] as 'round-robin' | 'random' | 'failover'
            if (!['round-robin', 'random', 'failover'].includes(strategy)) {
                console.error('错误: 不支持的策略，请使用 round-robin, random 或 failover')
                return
            }
            configManagerCLI.setLoadBalance(strategy)
            break

        default:
            console.error(`未知命令: ${command}`)
            showHelp()
    }

    // 命令执行完成后退出
    setTimeout(async () => {
        await redisCache.disconnect()
        process.exit(0)
    }, 100)
}

main()
