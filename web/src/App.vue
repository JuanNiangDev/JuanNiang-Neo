<template>
  <v-app theme="JuanNiangThemeDark">
    <RouterView />

    <v-snackbar v-if="toastStore.show" v-model="toastStore.show" :color="toastStore.current?.color"
      :timeout="toastStore.current?.timeout || 3000" location="top right" close-on-back>
      <div class="d-flex align-center">
        <v-icon v-if="toastStore.current?.color === 'success'" class="me-2" size="18">mdi-check-circle</v-icon>
        <v-icon v-else-if="toastStore.current?.color === 'error'" class="me-2" size="18">mdi-alert-circle</v-icon>
        <v-icon v-else-if="toastStore.current?.color === 'warning'" class="me-2" size="18">mdi-alert</v-icon>
        <v-icon v-else class="me-2" size="18">mdi-information</v-icon>
        <span>{{ toastStore.current?.message }}</span>
      </div>
      <template #actions>
        <v-btn icon="mdi-close" variant="text" size="small" @click="toastStore.show = false" />
      </template>
    </v-snackbar>
  </v-app>
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import { useToastStore } from '@/stores/toast'

const toastStore = useToastStore()
</script>