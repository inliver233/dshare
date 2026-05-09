import { AppShell as MantineAppShell, Avatar, Badge, Box, Button, Code, Group, NavLink, Stack, Text, Title } from '@mantine/core'
import { KeyRound, LogOut, Settings, Shield, Trophy } from 'lucide-react'
import type { ReactNode } from 'react'
import type { PublicConfig, User } from '../types/api'
import { displayName } from '../utils/format'

type Props = {
  user: User
  config: PublicConfig | null
  isAdminPath: boolean
  proxyBaseURL: string
  onLogout: () => void
  children: ReactNode
}

export function AppShell({ user, config, isAdminPath, proxyBaseURL, onLogout, children }: Props) {
  const name = displayName(user)
  const baseURL = proxyBaseURL || window.location.origin
  const navItems = [
    { href: '/', label: '账号', icon: KeyRound, active: !isAdminPath },
    { href: '/rank', label: '排行榜', icon: Trophy, active: false },
    ...(user.role === 'admin' ? [{ href: '/admin', label: '管理员', icon: Settings, active: isAdminPath }] : []),
  ]

  return (
    <MantineAppShell navbar={{ width: 272, breakpoint: 'sm', collapsed: { mobile: false } }} padding={0} className="appShell">
      <MantineAppShell.Navbar className="sidebar">
        <Stack gap="md" h="100%" className="sidebarStack">
          <Box component="a" href="/" className="brandLink">
            <Group gap="sm" className="brand">
              <Box className="brandMark"><Shield /></Box>
              <Box>
                <Text fw={700} c="var(--color-text-title)">DShare</Text>
                <Text size="xs" c="var(--color-text-muted)">Oceanic relay console</Text>
              </Box>
            </Group>
          </Box>

          <Group gap="sm" className="profile">
            <Avatar radius="xl" color="brandBlue" variant="filled">{name.slice(0, 1).toUpperCase()}</Avatar>
            <Box className="profileText">
              <Text fw={600} size="sm" truncate>{name}</Text>
              <Text size="xs" c="var(--color-text-muted)">{user.role === 'admin' ? '管理员' : '用户'}</Text>
            </Box>
          </Group>

          <Stack gap={4} className="navList">
            {navItems.map(({ href, label, icon: Icon, active }) => (
              <NavLink
                key={href}
                href={href}
                active={active}
                label={label}
                leftSection={<Icon size={18} />}
                variant="light"
                className="navItem"
              />
            ))}
          </Stack>

          <Button className="logoutButton" mt="auto" variant="light" color="gray" leftSection={<LogOut size={18} />} onClick={onLogout}>
            退出
          </Button>
        </Stack>
      </MantineAppShell.Navbar>

      <MantineAppShell.Main className="mainSurface">
        <Box className="mainContainer">
          <header className="pageHeader">
            <Box>
              <Title order={1}>{isAdminPath ? '管理员' : '账号'}</Title>
              <Group gap="xs" mt={8} className="proxyLine">
                <Text size="xs" c="var(--color-text-muted)">代理地址</Text>
                <Code className="codeText">{baseURL}</Code>
              </Group>
            </Box>
            <Group gap="xs" className="statusBadges">
              <Badge color={config?.new_api_enabled ? 'green' : 'red'} variant="light">new-api</Badge>
              <Badge color={config?.ds2api_enabled ? 'green' : 'red'} variant="light">ds2api</Badge>
            </Group>
          </header>
          {children}
        </Box>
      </MantineAppShell.Main>
    </MantineAppShell>
  )
}
