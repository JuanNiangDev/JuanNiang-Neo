<template>
  <button
    type="button"
    class="jn-toggle"
    :class="{ on: !!modelValue, off: !modelValue }"
    :disabled="disabled"
    @click.stop="$emit('update:modelValue', !modelValue)"
  >
    <span class="jn-toggle-dot" />
    <span class="jn-toggle-text">{{ modelValue ? '已启用' : '已停用' }}</span>
  </button>
</template>

<script setup lang="ts">
defineProps<{ modelValue: boolean; disabled?: boolean }>()
defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()
</script>

<style scoped>
.jn-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px 4px 8px;
  border-radius: 999px;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.22);
  background: transparent;
  color: rgba(var(--v-theme-on-surface), 0.62);
  font-size: 12px;
  line-height: 1;
  cursor: pointer;
  user-select: none;
  transition: all 0.18s ease;
}
.jn-toggle:hover:not(:disabled) {
  border-color: rgba(var(--v-theme-on-surface), 0.45);
  color: rgba(var(--v-theme-on-surface), 0.8);
}
.jn-toggle:active:not(:disabled) { transform: scale(0.95); }
.jn-toggle:disabled { opacity: 0.45; cursor: not-allowed; }

.jn-toggle-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: rgba(var(--v-theme-on-surface), 0.35);
  transition: all 0.18s ease;
}

/* 启用态 */
.jn-toggle.on {
  background: rgba(var(--v-theme-success), 0.12);
  border-color: rgba(var(--v-theme-success), 0.55);
  color: rgb(var(--v-theme-success));
}
.jn-toggle.on .jn-toggle-dot {
  background: rgb(var(--v-theme-success));
  box-shadow: 0 0 0 3px rgba(var(--v-theme-success), 0.22);
}

/* 停用态 */
.jn-toggle.off:hover:not(:disabled) {
  border-color: rgba(var(--v-theme-error), 0.5);
  color: rgb(var(--v-theme-error));
}
.jn-toggle.off:hover:not(:disabled) .jn-toggle-dot { background: rgb(var(--v-theme-error)); }
</style>
