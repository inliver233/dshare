export type User = {
  id: number
  discord_id: string
  discord_username: string
  discord_global_name: string
  discord_avatar: string
  role: string
  valid_uploads: number
  total_requests: number
  requests_per_minute: number
  requests_per_day: number
  max_concurrent_requests: number
}

export type APIKey = {
  id: number
  name: string
  prefix: string
  masked_key: string
  total_requests: number
  requests_today: number
  created_at: string
  last_used_at?: string
  revoked_at?: string
  key?: string
}

export type Contribution = {
  id: number
  account: string
  status: string
  message: string
  created_at: string
  response_time_ms?: number
}

export type MeResponse = {
  user: User
  stats: {
    valid_uploads: number
    total_requests: number
    requests_today: number
    requests_remaining: number
  }
  keys: APIKey[]
  proxy_base_url: string
}

export type PublicConfig = {
  discord_enabled: boolean
  ds2api_enabled: boolean
  new_api_enabled: boolean
  base_url: string
}

export type AdminStats = {
  users: number
  valid_uploads: number
  total_requests: number
  active_api_keys: number
}

export type ServiceConfig = {
  new_api_base_url: string
  new_api_key?: string
  new_api_key_preview: string
  ds2api_base_url: string
  ds2api_admin_key?: string
  ds2api_admin_key_preview: string
  ds2api_auto_proxy: {
    enabled: boolean
    type: string
    host: string
    port: number
    username_template: string
    password?: string
    password_preview: string
    name_template: string
  }
  discord_client_id: string
  discord_client_secret?: string
  discord_secret_preview: string
  discord_redirect_url: string
  app_base_url: string
}

export type ImportResult = {
  account: string
  status: string
  message: string
  response_time_ms?: number
}

export type ImportResponse = {
  total: number
  valid: number
  invalid: number
  duplicate: number
  results: ImportResult[]
}

export type RankBoard = 'requests' | 'contributions'
export type RankPeriod = 'all' | 'week' | 'day'

export type RankItem = {
  rank: number
  user_id: number
  display_name: string
  discord_username: string
  discord_id_preview: string
  value: number
}

export type RankResponse = {
  board: RankBoard
  period: RankPeriod
  items: RankItem[]
  limit: number
  offset: number
  next_offset?: number
  has_more: boolean
  generated_at: string
}
