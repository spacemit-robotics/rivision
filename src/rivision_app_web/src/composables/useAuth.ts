import { computed } from 'vue'
import { useRouter } from 'vue-router'

export function useAuth() {
  const router = useRouter()

  const isLoggedIn = computed(() => !!localStorage.getItem('rivision_token'))

  function getToken(): string | null {
    return localStorage.getItem('rivision_token')
  }

  function setToken(token: string) {
    localStorage.setItem('rivision_token', token)
  }

  function logout() {
    localStorage.removeItem('rivision_token')
    router.push('/login')
  }

  function requireAuth() {
    if (!isLoggedIn.value) {
      router.push('/login')
    }
  }

  return { isLoggedIn, getToken, setToken, logout, requireAuth }
}
