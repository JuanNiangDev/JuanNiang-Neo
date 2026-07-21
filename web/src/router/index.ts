import { createRouter, createWebHashHistory } from 'vue-router'
import DefaultLayout from '@/layouts/DefaultLayout.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/LoginPage.vue'),
      meta: { requiresAuth: false },
    },
    {
      path: '/',
      component: DefaultLayout,
      meta: { requiresAuth: true },
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', name: 'Dashboard', component: () => import('@/views/DashboardPage.vue') },
        { path: 'adapter', name: 'Adapter', component: () => import('@/views/AdapterPage.vue') },
        { path: 'providers', name: 'Providers', component: () => import('@/views/ProvidersPage.vue') },
        { path: 'mcp', name: 'MCP', component: () => import('@/views/MCPPage.vue') },
        { path: 'plugins', name: 'Plugins', component: () => import('@/views/PluginsPage.vue') },
        { path: 'prompts', name: 'Prompts', component: () => import('@/views/PromptsPage.vue') },
        { path: 'skills', name: 'Skills', component: () => import('@/views/SkillsPage.vue') },
        { path: 'sessions', name: 'Sessions', component: () => import('@/views/SessionsPage.vue') },
        { path: 'tools', name: 'Tools', component: () => import('@/views/ToolsPage.vue') },
        { path: 'acl', name: 'ACL', component: () => import('@/views/ACLPage.vue') },
        { path: 'chat-areas', name: 'ChatAreas', component: () => import('@/views/ChatAreasPage.vue') },
        { path: 'chat-records', name: 'ChatRecords', component: () => import('@/views/ChatRecordsPage.vue') },
        { path: 'memory', name: 'Memory', component: () => import('@/views/MemoryPage.vue') },
        { path: 't2i', name: 'T2I', component: () => import('@/views/T2IPage.vue') },
        { path: 'sandbox', name: 'Sandbox', component: () => import('@/views/SandboxPage.vue') },
        { path: 'webhook', name: 'Webhook', component: () => import('@/views/WebhookPage.vue') },
        { path: 'logs', name: 'Logs', component: () => import('@/views/LogsPage.vue') },
        { path: 'settings', name: 'Settings', component: () => import('@/views/SettingsPage.vue') },
        { path: 'background-tasks', name: 'BackgroundTasks', component: () => import('@/views/BackgroundTasksPage.vue') },
        { path: 'cronjobs', name: 'CronJobs', component: () => import('@/views/CronJobsPage.vue') },
      ],
    },
  ],
})

router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('token')
  if (to.meta.requiresAuth !== false && !token) {
    return next('/login')
  }
  if (to.path === '/login' && token) {
    return next('/dashboard')
  }
  next()
})

export default router
