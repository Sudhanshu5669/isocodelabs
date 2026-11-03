import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '../api/client'

export type Loadable<T> = {
  data: T | null
  error: string
  loading: boolean
  /** Fetch again, e.g. after saving a change. */
  reload: () => void
}

/**
 * Loads `path` from the API and tracks loading and error state, so every
 * screen handles "loading", "failed" and "here it is" the same way.
 * Pass null to load nothing (yet).
 *
 * When `path` changes, the old answer is dropped (so a page never shows the
 * previous lead or project under the new heading) unless `keepPrevious` is
 * set: lists that are re-filtered keep showing the old rows, faded, until the
 * new ones arrive.
 */
export function useApi<T>(path: string | null, { keepPrevious = false } = {}): Loadable<T> {
  const [loaded, setLoaded] = useState<{ path: string; data: T } | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(path !== null)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    if (path === null) {
      setLoading(false)
      return
    }
    const controller = new AbortController()
    setLoading(true)
    setError('')
    api<T>(path, { signal: controller.signal })
      .then((d) => setLoaded({ path, data: d }))
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        setError(err instanceof ApiError ? err.message : 'Something went wrong. Please try again.')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [path, tick])

  const reload = useCallback(() => setTick((t) => t + 1), [])
  const data = loaded && (keepPrevious || loaded.path === path) ? loaded.data : null
  return { data, error, loading, reload }
}

/** The message to show for any error thrown by api(). */
export function errorText(err: unknown): string {
  return err instanceof ApiError ? err.message : 'Something went wrong. Please try again.'
}
