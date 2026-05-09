import type { User } from '../types/api'

export function displayName(user: Pick<User, 'discord_global_name' | 'discord_username' | 'discord_id'>) {
  return user.discord_global_name || user.discord_username || user.discord_id
}

export function formatDate(value?: string) {
  if (!value) return ''
  return new Date(value).toLocaleString()
}

export function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}
