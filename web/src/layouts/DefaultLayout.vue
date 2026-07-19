<template>
  <v-layout>
    <!-- Sidebar -->
    <AppSidebar />

    <!-- Header -->
    <AppHeader />

    <!-- Main Content -->
    <v-main>
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
