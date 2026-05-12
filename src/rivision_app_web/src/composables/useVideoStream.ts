import { ref } from 'vue'

export function useVideoStream() {
  const protocol = ref<'webrtc' | 'hls' | 'flv'>('webrtc')
  const playing = ref(false)
  const error = ref<string | null>(null)

  async function startWebRTC(videoEl: HTMLVideoElement, streamUrl: string) {
    // WebRTC via go2rtc API
    // In production: POST go2rtc /api/webrtc?src={streamUrl}
    protocol.value = 'webrtc'
    playing.value = true
    error.value = null
  }

  function stop(videoEl: HTMLVideoElement) {
    videoEl.srcObject = null
    videoEl.src = ''
    playing.value = false
  }

  return { protocol, playing, error, startWebRTC, stop }
}
