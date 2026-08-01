import type { Plugin, ViteDevServer } from 'vite'
import { mockHandlers } from './handlers'

export function mockPlugin(): Plugin {
  return {
    name: 'vite-mock-plugin',
    configureServer(server: ViteDevServer) {
      const MOCK_ENABLED = process.env.VITE_MOCK === 'true'
      if (!MOCK_ENABLED) return

      console.log('🔧 Mock mode enabled — intercepting /api/v1/* requests')

      // Use a catch-all middleware to intercept API requests
      server.middlewares.use(async (req, res, next) => {
        try {
          const url = new URL(req.url || '/', `http://127.0.0.1`)
          const fullPath = url.pathname

          // Only handle /api/v1/ requests
          if (!fullPath.startsWith('/api/v1/')) {
            return next()
          }

          const pathname = fullPath.replace(/^\/api\/v1/, '')
          const method = req.method?.toUpperCase() || 'GET'

          // Parse body for POST/PUT requests
          let body: any = null
          if (['POST', 'PUT', 'PATCH'].includes(method)) {
            body = await parseBody(req)
          }

          // Find matching handler
          const handler = mockHandlers.find(
            (h) => h.method === method && matchRoute(h.path, pathname)
          )

          if (handler) {
            const params = extractParams(handler.path, pathname)
            const query = Object.fromEntries(url.searchParams.entries())

            // Simulate network delay
            await delay(100 + Math.random() * 200)

            const result = handler.handler({ params, query, body })
            res.setHeader('Content-Type', 'application/json')
            res.setHeader('Access-Control-Allow-Origin', '*')
            res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type')
            res.statusCode = 200
            res.end(JSON.stringify(result))
            return
          }

          // Handle OPTIONS preflight
          if (method === 'OPTIONS') {
            res.setHeader('Access-Control-Allow-Origin', '*')
            res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type')
            res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS')
            res.statusCode = 204
            res.end()
            return
          }

          // No handler found, let Vite fallback to proxy or 404
          next()
        } catch (e: any) {
          console.error('[mock] handler error:', e.message || e)
          res.statusCode = 500
          res.setHeader('Content-Type', 'application/json')
          res.end(JSON.stringify({ status: 50000, info: 'Mock internal error: ' + (e.message || 'unknown'), data: null }))
        }
      })
    },
  }
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}

function matchRoute(pattern: string, pathname: string): boolean {
  const patternParts = pattern.split('/')
  const pathParts = pathname.split('/')

  if (patternParts.length !== pathParts.length) return false

  for (let i = 0; i < patternParts.length; i++) {
    if (patternParts[i].startsWith(':')) continue
    if (patternParts[i] !== pathParts[i]) return false
  }

  return true
}

function extractParams(pattern: string, pathname: string): Record<string, string> {
  const params: Record<string, string> = {}
  const patternParts = pattern.split('/')
  const pathParts = pathname.split('/')

  for (let i = 0; i < patternParts.length; i++) {
    if (patternParts[i].startsWith(':')) {
      params[patternParts[i].slice(1)] = pathParts[i]
    }
  }

  return params
}

function parseBody(req: any): Promise<any> {
  return new Promise((resolve) => {
    // Vite 可能已解析 body，优先使用
    if (req.body) {
      return resolve(req.body)
    }
    // 尝试读取已缓存的 raw body
    if (req._body) {
      try { return resolve(JSON.parse(req._body)) } catch { return resolve({}) }
    }
    const chunks: Buffer[] = []
    req.on('data', (chunk: Buffer) => chunks.push(chunk))
    req.on('end', () => {
      const raw = Buffer.concat(chunks).toString()
      try {
        resolve(JSON.parse(raw))
      } catch {
        resolve({})
      }
    })
    // 超时兜底：如果 500ms 内没有 data 事件，body 已被消费
    setTimeout(() => resolve({}), 500)
  })
}

export interface MockContext {
  params: Record<string, string>
  query: Record<string, string>
  body: any
}

export interface MockHandler {
  method: string
  path: string
  handler: (ctx: MockContext) => any
}
