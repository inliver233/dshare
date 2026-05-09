import { AppShell as MantineAppShell, Avatar, Badge, Box, Button, Code, Group, NavLink, Stack, Text, Title } from '@mantine/core'
import { KeyRound, LogOut, Settings, Shield, Trophy } from 'lucide-react'
import { motion, AnimatePresence } from 'framer-motion'
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
            {navItems.map(({ href, label, icon: Icon, active }, i) => (
              <motion.div
                key={href}
                initial={{ opacity: 0, x: -10 }}
                animate={{ opacity: 1, x: 0 }}
                transition={{ delay: 0.1 + i * 0.05, type: 'spring', stiffness: 300, damping: 30 }}
              >
                <NavLink
                  href={href}
                  active={active}
                  label={label}
                  leftSection={<Icon size={18} />}
                  variant="light"
                  className="navItem"
                />
              </motion.div>
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
              <Group gap="md" align="center">
                <Title order={1}>{isAdminPath ? '管理员' : '账号'}</Title>
                <Group gap="xs" className="statusBadges">
                  <Badge bg="rgba(13, 148, 136, 0.1)" c="#0D9488" radius={99} size="sm" style={{ fontWeight: 800 }}>new-api</Badge>
                  <Badge bg="rgba(13, 148, 136, 0.1)" c="#0D9488" radius={99} size="sm" style={{ fontWeight: 800 }}>ds2api</Badge>
                </Group>
              </Group>
              <Group gap="xs" mt={8} className="proxyLine">
                <Text size="xs" c="var(--color-text-muted)">代理地址</Text>
                <Code className="codeText">{baseURL}</Code>
              </Group>
            </Box>
          </header>
          
          <AnimatePresence mode="wait">
            <motion.div
              key={isAdminPath ? 'admin' : 'dashboard'}
              initial={{ opacity: 0, y: 15 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -15 }}
              transition={{ type: 'spring', stiffness: 400, damping: 40 }}
            >
              {children}
            </motion.div>
          </AnimatePresence>
        </Box>
      </MantineAppShell.Main>
    </MantineAppShell>
  )
}
