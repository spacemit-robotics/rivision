import { ref, onUnmounted } from 'vue'
import { wsClient } from '../api/ws'

export function useWebSocket() {
  const connected = ref(false)
  const lastEvent = ref<any>(null)

  function connect() {
    wsClient.connect()
    connected.value = true
  }

  function on(type: string, handler: (event: any) => void) {
    wsClient.on(type, handler)
  }

  function off(type: string, handler: (event: any) => void) {
    wsClient.off(type, handler)
  }

  onUnmounted(() => {
    // Don't close the shared client on unmount — it's a singleton
  })

  return { connected, lastEvent, connect, on, off }
}
