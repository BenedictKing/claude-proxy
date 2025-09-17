#!/usr/bin/env node
import { configManagerCLI, UpstreamConfig } from './src/config'
import { maskApiKey } from './src/utils'

function showHelp() {
  console.log(`
Claude API代理服务器配置工具

使用方法:
  bun run config <command> [options]

命令:
  show                    显示当前配置
  add <name> <type> <url> 添加上游配置
  remove <index|name>     删除上游配置
  update <index|name> [options] 更新上游配置
  use <index|name>        切换到指定上游（支持索引或名称）
  key <index|name> <action>    管理API密钥
  balance <strategy>      设置负载均衡策略
  help                    显示帮助信息

参数:
  name                    上游名称
  type                    服务类型 (gemini, openaiold, openai, claude)
  url                     上游基础URL
  index                   上游索引或名称
  action                  密钥操作 (add, remove, list)
  strategy                负载均衡策略 (round-robin, random, failover)

示例:
  bun run config show                              # 显示当前配置
  bun run config add MyGemini gemini https://generativelanguage.googleapis.com/v1beta
  bun run config add MyOpenAI openai https://api.openai.com/v1
  bun run config use 0                             # 切换到第一个上游
  bun run config use MyOpenAI                      # 切换到名称为 MyOpenAI 的上游
  bun run config key 0 add sk-1234567890abcdef      # 为索引为0的上游添加API密钥
  bun run config key MyOpenAI list                 # 列出名为 MyOpenAI 的上游的API密钥
  bun run config update 1 --name "NewName"         # 更新索引为1的上游名称
  bun run config update MyOpenAI --description "备注" # 更新名为 MyOpenAI 的上游的备注
  bun run config update MyOpenAI --insecureSkipVerify true # 开启跳过TLS验证
  bun run config balance round-robin               # 设置负载均衡
  bun run config remove 0                          # 按索引删除上游
  bun run config remove MyOpenAI                   # 按名称删除上游

支持的默认URL:
  gemini: https://generativelanguage.googleapis.com/v1beta
  openaiold: https://api.openai.com/v1
  openai: https://api.openai.com/v1
  claude: https://api.anthropic.com/v1
`)
}

function parseArgs(args: string[]): { [key: string]: string } {
  return args.reduce((acc, arg, i) => {
    const match = arg.match(/^--(.+)/)
    if (match) {
      const key = match[1]
      const nextArg = args[i + 1]
      acc[key] = nextArg && !nextArg.startsWith('--') ? nextArg : 'true'
    }
    return acc
  }, {} as { [key: string]: string })
}

function addUpstream(name: string, type: string, url: string, extraArgs?: string[]) {
  if (!['gemini', 'openaiold', 'openai', 'claude'].includes(type)) {
    console.error('错误: 不支持的类型，请使用 gemini, openaiold, openai, 或 claude')
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
    if (flags.insecureSkipVerify) {
      upstream.insecureSkipVerify = flags.insecureSkipVerify === 'true'
    }
  }

  configManagerCLI.addUpstream(upstream)
}

function updateUpstream(indexOrName: string, args: string[]) {
  const flags = parseArgs(args)

  const update: Partial<UpstreamConfig> = {}
  if (flags.name) update.name = flags.name
  if (flags.url) update.baseUrl = flags.url
  if (flags.type) update.serviceType = flags.type as any
  if (flags.description) update.description = flags.description
  if (flags.insecureSkipVerify) {
    update.insecureSkipVerify = flags.insecureSkipVerify === 'true'
  }

  if (Object.keys(update).length === 0) {
    console.error('错误: 请指定要更新的字段 (--name, --url, --type, --description, --insecureSkipVerify)')
    return
  }

  configManagerCLI.updateUpstream(indexOrName, update)
}

function manageKeys(indexOrName: string, action: string, args: string[]) {
  switch (action) {
    case 'add':
      const apiKey = args[0]
      if (!apiKey) {
        console.error('错误: 请提供API密钥')
        return
      }
      configManagerCLI.addApiKey(indexOrName, apiKey)
      break

    case 'remove':
      const keyToRemove = args[0]
      if (!keyToRemove) {
        console.error('错误: 请提供要删除的API密钥')
        return
      }
      configManagerCLI.removeApiKey(indexOrName, keyToRemove)
      break

    case 'list':
      const index = configManagerCLI.findUpstreamIndex(indexOrName)
      const config = configManagerCLI.getConfig()
      const upstream = config.upstream[index]
      console.log(`上游 [${index}] "${upstream.name || upstream.serviceType}" 的API密钥(已脱敏):`)
      if (upstream.apiKeys.length === 0) {
        console.log('  没有API密钥')
      } else {
        upstream.apiKeys.forEach((key, i) => {
          console.log(`  [${i}] ${maskApiKey(key)}`)
        })
      }
      break

    default:
      console.error('错误: 不支持的密钥操作，请使用 add, remove 或 list')
  }
}

function main() {
  const run = () => {
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
          console.log(
            '      bun run config add MyOpenAI openaiold https://api.openai.com/v1 --description "官方OpenAI接口"'
          )
          return
        }
        addUpstream(args[1], args[2], args[3], args.slice(4))
        break

      case 'remove':
        if (args.length < 2) {
          console.error('错误: remove 命令需要 index 或 name 参数')
          return
        }
        try {
          configManagerCLI.removeUpstream(args[1])
        } catch (error) {
          console.error(error instanceof Error ? error.message : String(error))
        }
        break

      case 'update':
        if (args.length < 2) {
          console.error('错误: update 命令需要 index 或 name 参数')
          return
        }
        try {
          updateUpstream(args[1], args.slice(2))
        } catch (error) {
          console.error(error instanceof Error ? error.message : String(error))
        }
        break

      case 'use':
        if (args.length < 2) {
          console.error('错误: use 命令需要 index 或 name 参数')
          return
        }
        try {
          configManagerCLI.setUpstream(args[1])
        } catch (error) {
          console.error(`错误: ${error instanceof Error ? error.message : error}`)
        }
        break

      case 'key':
        if (args.length < 3) {
          console.error('错误: key 命令需要 index/name 和 action 参数')
          return
        }
        try {
          manageKeys(args[1], args[2], args.slice(3))
        } catch (error) {
          console.error(error instanceof Error ? error.message : String(error))
        }
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
  }

  run()

  // 命令执行完成后退出
  setTimeout(() => {
    process.exit(0)
  }, 100)
}

main()
