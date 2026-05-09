import { useState } from 'react'
import { Anchor, Box, Button, Card, Group, PasswordInput, Stack, Text, TextInput, ThemeIcon, Title } from '@mantine/core'
import { ArrowRight, KeyRound, LogIn, LogOut, Shield, Sparkles } from 'lucide-react'
import type { PublicConfig } from '../types/api'
import { api } from '../services/api'
import { ErrorText } from '../components/Feedback'

export function PublicLogin({ config, message }: { config: PublicConfig | null; message: string }) {
  return (
    <Box className="loginPage">
      <Card className="loginCard">
        <Stack gap="md">
          <Group gap="sm">
            <ThemeIcon size={40} radius="md"><Shield size={22} /></ThemeIcon>
            <Box>
              <Text fw={700}>DShare</Text>
              <Text size="xs" c="var(--color-text-muted)">Oceanic relay console</Text>
            </Box>
          </Group>
          <Title order={1}>安全的 DeepSeek 共享中继</Title>
          <Text c="var(--color-text-body)" size="sm">使用 Discord 登录后获取项目 API Key，提交 DeepSeek 账号贡献到账号池，并自动提升调用额度。</Text>
          <Group gap="xs">
            <Text className="featurePill"><KeyRound size={14} />API Key</Text>
            <Text className="featurePill"><Sparkles size={14} />自动验证</Text>
          </Group>
          {config?.discord_enabled ? (
            <Button component="a" href="/api/auth/discord/start" fullWidth leftSection={<LogIn size={18} />} rightSection={<ArrowRight size={18} />}>Discord 登录</Button>
          ) : (
            <ErrorText>Discord 登录尚未配置，请联系管理员。</ErrorText>
          )}
          <ErrorText>{message}</ErrorText>
        </Stack>
      </Card>
    </Box>
  )
}

export function AdminLogin({ onLogin, message }: { onLogin: () => void; message: string }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState(message)
  const adminLogin = async () => {
    setErr('')
    try {
      await api('/api/auth/admin/login', { method: 'POST', body: JSON.stringify({ username, password }) })
      onLogin()
    } catch (e) {
      setErr((e as Error).message)
    }
  }
  return (
    <Box className="loginPage">
      <Card className="loginCard">
        <Stack gap="md">
          <Group gap="sm">
            <ThemeIcon size={40} radius="md"><Shield size={22} /></ThemeIcon>
            <Box>
              <Text fw={700}>DShare Admin</Text>
              <Text size="xs" c="var(--color-text-muted)">管理员入口</Text>
            </Box>
          </Group>
          <Title order={1}>管理员登录</Title>
          <Text c="var(--color-text-body)" size="sm">管理员入口用于查看用户贡献、调用量，并修改每个用户的请求限制。</Text>
          <TextInput placeholder="管理员账号" value={username} onChange={(e) => setUsername(e.currentTarget.value)} />
          <PasswordInput placeholder="管理员密码" value={password} onChange={(e) => setPassword(e.currentTarget.value)} />
          <Button onClick={adminLogin} leftSection={<Shield size={18} />}>管理员登录</Button>
          <ErrorText>{err}</ErrorText>
        </Stack>
      </Card>
    </Box>
  )
}

export function Forbidden({ onLogout }: { onLogout: () => void }) {
  return (
    <Box className="loginPage">
      <Card className="loginCard">
        <Stack gap="md">
          <ThemeIcon size={40} radius="md"><Shield size={22} /></ThemeIcon>
          <Title order={1}>无管理员权限</Title>
          <Text c="var(--color-text-body)" size="sm">当前登录账号不是管理员。请退出后使用管理员账号登录。</Text>
          <Group>
            <Button onClick={onLogout} leftSection={<LogOut size={18} />}>退出</Button>
            <Anchor href="/">返回账号页</Anchor>
          </Group>
        </Stack>
      </Card>
    </Box>
  )
}
