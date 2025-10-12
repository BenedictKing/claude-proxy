import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'
import { resolve } from 'path'

export default defineConfig(({ mode }) => {
  // 加载环境变量
  const env = loadEnv(mode, process.cwd(), '')

  const frontendPort = parseInt(env.VITE_FRONTEND_PORT || '5173')
  const backendUrl = env.VITE_BACKEND_URL || 'http://localhost:3000'

  return {
    // 使用绝对路径，适配 Go 嵌入式部署
    base: '/',

    plugins: [
      vue(),
      vuetify({
        autoImport: true,
        styles: {
          configFile: 'src/styles/settings.scss'
        },
        theme: {
          defaultTheme: 'light'
        }
      })
    ],
    resolve: {
      alias: {
        '@': resolve(__dirname, 'src')
      }
    },
    server: {
      port: frontendPort,
      proxy: {
        '/api': {
          target: backendUrl,
          changeOrigin: true
        },
        '/v1': {
          target: backendUrl,
          changeOrigin: true
        }
      }
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
      // 确保资源路径正确
      assetsDir: 'assets',
      // 优化代码分割
      rollupOptions: {
        output: {
          manualChunks: {
            'vue-vendor': ['vue', 'vuetify']
          }
        }
      }
    }
  }
})
