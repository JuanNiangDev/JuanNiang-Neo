<template>
  <v-navigation-drawer
    v-model="appStore.sidebarOpen"
    app
    width="260"
    elevation="0"
    class="app-sidebar overflow-hidden"
    color="sidebar-bg"
    :temporary="$vuetify.display.mobile"
  >
    <div class="d-flex align-center ps-4 pe-3 py-3" style="min-height: 64px; gap: 10px; flex-shrink: 0">
      <img v-if="logoExists" src="/site_logo.png" alt="Logo" style="height: 30px; width: 30px; border-radius: 8px; flex-shrink: 0" @error="logoExists = false" />
      <div class="d-flex flex-column" style="line-height: 1.1">
        <span class="font-weight-bold" style="color: rgba(255,255,255,0.92); font-size: 15px; white-space: nowrap">JuanNiang-Neo</span>
        <span class="text-caption" style="color: rgba(255,255,255,0.35); font-size: 11px">智能管理控制台</span>
      </div>
    </div>

    <v-divider class="mx-3" style="border-color: rgba(255,255,255,0.06); flex-shrink: 0" />

    <!-- Collapsible nav groups -->
     <!-- Add scroll space-->
      <div class="sidebar-scroll">
    <v-list nav density="compact" class="sidebar-nav" color="sidebar-text" v-model:opened="openedGroups" open-strategy="multiple">
      <v-list-group
        v-for="group in navGroups"
        :key="group.title"
        :value="group.title"
      >
        <template #activator="{ props }">
          <v-list-item
            v-bind="props"
            :prepend-icon="group.icon"
            :title="group.title"
          />
        </template>

        <v-list-item
          v-for="item in group.items"
          :key="item.to"
          :to="item.to"
          :title="item.title"
          :prepend-icon="item.icon"
          active-color="primary"
          :exact="item.to === '/dashboard'"
        />
      </v-list-group>
    </v-list>
    </div>

    <div class="px-3 py-2 flex-shrink-0">
      <v-divider style="border-color: rgba(255,255,255,0.06)" class="mb-2" />
      <div class="text-caption px-3 py-2" style="color: rgba(255,255,255,0.28); font-size: 11px">
        JuanNiang-Neo v1.0.0
      </div>
    </div>
  </v-navigation-drawer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const logoExists = ref(true)

const openedGroups = ref<string[]>(['概览', '核心配置'])

const navGroups = [
  {
    title: '概览',
    icon: 'mdi-view-dashboard-outline',
    items: [
      { title: '仪表盘', icon: 'mdi-view-dashboard-outline', to: '/dashboard' },
      { title: '系统日志', icon: 'mdi-text-box-outline', to: '/logs' },
    ],
  },
  {
    title: '核心配置',
    icon: 'mdi-cog-outline',
    items: [
      { title: '回复设置', icon: 'mdi-reply-circle', to: '/reply-strategy' },
      { title: 'Adapter', icon: 'mdi-connection', to: '/adapter' },
      { title: 'LLM Providers', icon: 'mdi-brain', to: '/providers' },
      { title: 'MCP 服务器', icon: 'mdi-server-network', to: '/mcp' },
      { title: 'Webhook', icon: 'mdi-webhook', to: '/webhook' },
      { title: 'CronJob', icon: 'mdi-timer-cog-outline', to: '/cronjobs' },
    ],
  },
  {
    title: '智能体管理',
    icon: 'mdi-robot-outline',
    items: [
      { title: 'Prompts', icon: 'mdi-text-box-edit-outline', to: '/prompts' },
      { title: 'Skills', icon: 'mdi-lightning-bolt', to: '/skills' },
      { title: 'Tools', icon: 'mdi-tools', to: '/tools' },
      { title: 'Sessions', icon: 'mdi-chat-processing-outline', to: '/sessions' },
      { title: 'Memory', icon: 'mdi-memory', to: '/memory' },
      { title: '知识库', icon: 'mdi-book-open-variant', to: '/knowledge' },
      { title: 'Agent 循环', icon: 'mdi-brain', to: '/agent-loops' },
    ],
  },
  {
    title: 'Plugin 管理',
    icon: 'mdi-puzzle',
    items: [
      { title: 'Plugins', icon: 'mdi-puzzle', to: '/plugins' },
      { title: 'Plugin 商店', icon: 'mdi-storefront-outline', to: '/plugin-store' },
    ],
  },
  {
    title: '数据与安全',
    icon: 'mdi-shield-account-outline',
    items: [
      { title: 'Chat Areas', icon: 'mdi-forum-outline', to: '/chat-areas' },
      { title: 'Chat Records', icon: 'mdi-message-text-outline', to: '/chat-records' },
      { title: '黑名单', icon: 'mdi-shield-account-outline', to: '/acl' },
    ],
  },
	{
		title: '高级功能',
		icon: 'mdi-rocket-launch-outline',
		items: [
			{ title: '图床', icon: 'mdi-image-multiple-outline', to: '/advanced/image-host' },
			{ title: '表情包库', icon: 'mdi-emoticon-outline', to: '/advanced/stickers' },
			{ title: '摸鱼人日历', icon: 'mdi-calendar-month-outline', to: '/advanced/fish-calendar' },
			{ title: '定时消息', icon: 'mdi-message-text-clock-outline', to: '/advanced/scheduled-messages' },
		],
	},
	{
		title: '服务',
		icon: 'mdi-cog-outline',
		items: [
			{ title: 'T2I', icon: 'mdi-image-auto-adjust', to: '/t2i' },
			{ title: 'Sandbox', icon: 'mdi-code-braces-box', to: '/sandbox' },
			{ title: '修改密码', icon: 'mdi-key-change', to: '/settings' },
		],
	},
]
</script>
