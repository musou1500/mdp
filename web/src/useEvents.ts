import { useEffect } from 'react'

// Changes arrive as bursts, and the server has already coalesced them once.
const debounceMs = 50

// useEvents calls onChange whenever a file under the document root changes.
// EventSource reconnects on its own, so a dropped connection needs no handling.
export function useEvents(onChange: () => void) {
  useEffect(() => {
    let timer: number | undefined
    const source = new EventSource('/_md/events')
    source.onmessage = () => {
      clearTimeout(timer)
      timer = setTimeout(onChange, debounceMs)
    }
    return () => {
      clearTimeout(timer)
      source.close()
    }
  }, [onChange])
}
