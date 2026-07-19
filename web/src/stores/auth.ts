import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')
  const username = ref(localStorage.getItem('username') || '')
  const returnUrl = ref<string | null>(null)

  const isLoggedIn = computed(() => !!token.value)

  function has_token(): boolean {
    return !!token.value
  }

  async function login(user: string, pwd: string) {
    const res = await authApi.login({ username: user, password: pwd })
    const t = res.data.data.token
    token.value = t
    username.value = user
    localStorage.setItem('token', t)
    localStorage.setItem('username', user)
  }

  function logout() {
    token.value = ''
    username.value = ''
    localStorage.removeItem('token')
    localStorage.removeItem('username')
    returnUrl.value = null
  }

  async function changePassword(oldPwd: string, newPwd: string) {
    await authApi.changePassword({ old_password: oldPwd, new_password: newPwd })
  }

  return { token, username, returnUrl, isLoggedIn, has_token, login, logout, changePassword }
})
