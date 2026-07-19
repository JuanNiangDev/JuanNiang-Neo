<template>
  <v-app theme="JuanNiangThemeDark">
    <RouterView />

    <v-snackbar v-if="toastStore.show" v-model="toastStore.show" :color="toastStore.current?.color"
      :timeout="toastStore.current?.timeout || 3000" location="top right" close-on-back>
      {{ toastStore.current?.message }}
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
