<template>
  <v-layout style="height: 100vh">
    <!-- Sidebar -->
    <AppSidebar />

    <!-- Header -->
    <AppHeader />

    <!-- Main Content -->
    <v-main style="height: 100vh; overflow-y: auto">
      <v-container fluid class="page-wrapper">
        <RouterView />
      </v-container>
    </v-main>
  </v-layout>
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import AppHeader from './AppHeader.vue'
import AppSidebar from './AppSidebar.vue'
</script>

<style>
/* 确保 html/body/#app 高度全占, 不让默认 margin 撑出滚动条 */
html, body, #app {
  height: 100%;
  margin: 0;
  padding: 0;
}
</style>
