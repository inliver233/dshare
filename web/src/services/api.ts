export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
    ...init,
  })
  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }
  if (!res.ok) {
    const message = typeof data === 'object' && data !== null
      ? (data as { error?: { message?: string }; message?: string }).error?.message || (data as { message?: string }).message
      : typeof data === 'string' && data.trim()
        ? data
        : ''
    throw new Error(message || `HTTP ${res.status}`)
  }
  return data as T
}

export function queryString(values: Record<string, string | number | undefined>) {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) {
    if (value !== undefined && `${value}`.trim() !== '') {
      params.set(key, `${value}`)
    }
  }
  const out = params.toString()
  return out ? `?${out}` : ''
}
