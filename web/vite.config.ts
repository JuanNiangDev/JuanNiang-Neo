import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vuetify from 'vite-plugin-vuetify'
import { fileURLToPath, URL } from 'url'
import { mockPlugin } from './mock/plugin'

const isMock = process.env.VITE_MOCK === 'true'

export default defineConfig({
  plugins: [
    vue(),
    vuetify({ autoImport: true }),
    mockPlugin(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  css: {
    preprocessorOptions: {
      scss: {}
    }
  },
  build: {
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-vue': ['vue', 'vue-router', 'pinia'],
          'vendor-vuetify': ['vuetify'],
          'vendor-axios': ['axios'],
        },
      },
    },
  },
  server: {
    host: '0.0.0.0',
    port: 3000,
    // Only proxy when NOT in mock mode
    ...(isMock ? {} : {
      proxy: {
        '/api': {
          target: 'http://127.0.0.1:8090/',
          changeOrigin: true,
          ws: true
        }
      }
    })
  }
})
