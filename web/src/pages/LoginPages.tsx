import { useEffect, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { Anchor, Box, Button, Group, PasswordInput, Stack, Text, TextInput, ThemeIcon, Title, ActionIcon } from '@mantine/core'
import { ArrowRight, KeyRound, LogIn, LogOut, Shield, Sparkles, Sun, Moon, Sunrise, Sunset } from 'lucide-react'
import { motion, Variants } from 'framer-motion'
import type { PublicConfig } from '../types/api'
import { api } from '../services/api'
import { ErrorText } from '../components/Feedback'
import { OceanCanvas } from '../components/OceanCanvas'
import { GlassPanel } from '../components/GlassPanel'
import { useOceanState } from '../hooks/useOceanState'
import { TimeStateKey } from '../theme/timeOfDay'

const cardVariants: Variants = {
  hidden: { opacity: 0, y: 30, scale: 0.95 },
  visible: { opacity: 1, y: 0, scale: 1, transition: { type: 'spring', stiffness: 300, damping: 25 } },
}

const TIME_OPTIONS: { key: TimeStateKey; icon: ReactNode; label: string; color: string }[] = [
  { key: 'dawn', icon: <Sunrise size={20} />, label: '黎明', color: '#F39C12' },
  { key: 'day', icon: <Sun size={20} />, label: '白昼', color: '#48CAE4' },
  { key: 'dusk', icon: <Sunset size={20} />, label: '黄昏', color: '#E74C3C' },
  { key: 'night', icon: <Moon size={20} />, label: '星夜', color: '#90E0EF' },
]

function OceanLayout({ children }: { children: ReactNode }) {
  const timeOfDay = useOceanState(s => s.timeOfDay)
  const setTimeOfDay = useOceanState(s => s.setTimeOfDay)

  return (
    <Box className="oceanLoginLayout" style={{ position: 'relative', width: '100vw', height: '100vh', overflow: 'hidden' }}>
      <OceanCanvas />
      
      <Box className="timeSwitcher" style={{ position: 'absolute', right: 24, top: '50%', transform: 'translateY(-50%)', zIndex: 10, display: 'flex', flexDirection: 'column', gap: 12 }}>
        {TIME_OPTIONS.map(opt => (
          <motion.div key={opt.key} whileHover={{ scale: 1.1 }} whileTap={{ scale: 0.95 }}>
            <ActionIcon
              size={48}
              radius="xl"
              onClick={() => setTimeOfDay(opt.key)}
              style={{
                border: timeOfDay === opt.key ? `2px solid ${opt.color}` : '2px solid rgba(255,255,255,0.2)',
                background: timeOfDay === opt.key ? `${opt.color}22` : 'rgba(255,255,255,0.05)',
                backdropFilter: 'blur(8px)',
                color: timeOfDay === opt.key ? opt.color : 'rgba(255,255,255,0.7)',
                transition: 'all 0.3s ease'
              }}
              title={opt.label}
              aria-label={opt.label}
              aria-pressed={timeOfDay === opt.key}
            >
              {opt.icon}
            </ActionIcon>
          </motion.div>
        ))}
      </Box>

      <Box className="oceanLoginContent" style={{ position: 'absolute', inset: 0, zIndex: 5, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        {children}
      </Box>
    </Box>
  )
}

export function PublicLogin({ config, message }: { config: PublicConfig | null; message: string }) {
  return (
    <OceanLayout>
      <motion.div className="loginPanelFrame" initial="hidden" animate="visible" variants={cardVariants} style={{ width: '100%', maxWidth: 440, padding: '20px' }}>
        <GlassPanel>
          <Stack gap="xl">
            <Group gap="sm" justify="center">
              <ThemeIcon size={48} radius="xl" variant="gradient" gradient={{ from: '#48CAE4', to: '#0077B6' }}>
                <Shield size={24} />
              </ThemeIcon>
            </Group>
            
            <Box style={{ textAlign: 'center' }}>
              <Title order={1} style={{ color: 'white', textShadow: '0 0 20px rgba(72,202,228,0.5)', marginBottom: 8 }}>Deepseek大锅饭</Title>
              <Text size="sm" style={{ color: 'rgba(255,255,255,0.7)' }}>安全的 DeepSeek 共享控制台</Text>
            </Box>

            <Group gap="xs" justify="center">
              <Box style={{ background: 'rgba(255,255,255,0.1)', padding: '4px 12px', borderRadius: 999, display: 'flex', alignItems: 'center', gap: 6 }}>
                <KeyRound size={14} color="#48CAE4" />
                <Text size="xs" c="white">API Key 分发</Text>
              </Box>
              <Box style={{ background: 'rgba(255,255,255,0.1)', padding: '4px 12px', borderRadius: 999, display: 'flex', alignItems: 'center', gap: 6 }}>
                <Sparkles size={14} color="#48CAE4" />
                <Text size="xs" c="white">自动验证</Text>
              </Box>
            </Group>

            {config?.discord_enabled ? (
              <Button 
                component="a" 
                href="/api/auth/discord/start" 
                aria-label="Discord 登录"
                fullWidth 
                leftSection={<LogIn size={18} />} 
                rightSection={<ArrowRight size={18} />} 
                size="xl"
                variant="gradient"
                gradient={{ from: '#0077B6', to: '#023E8A' }}
                style={{
                  boxShadow: '0 4px 15px rgba(0, 119, 182, 0.4)',
                  border: '1px solid rgba(72,202,228,0.3)',
                }}
              >
                Discord 登录
              </Button>
            ) : (
              <ErrorText>Discord 登录尚未配置，请联系管理员。</ErrorText>
            )}
            <ErrorText>{message}</ErrorText>
          </Stack>
        </GlassPanel>
      </motion.div>
    </OceanLayout>
  )
}

export function AdminLogin({ onLogin, message }: { onLogin: () => void; message: string }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState(message)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    setErr(message)
  }, [message])
  
  const adminLogin = async (event?: FormEvent<HTMLFormElement>) => {
    event?.preventDefault()
    if (submitting) return
    setErr('')
    setSubmitting(true)
    try {
      await api('/api/auth/admin/login', { method: 'POST', body: JSON.stringify({ username, password }) })
      onLogin()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <OceanLayout>
      <motion.div className="loginPanelFrame" initial="hidden" animate="visible" variants={cardVariants} style={{ width: '100%', maxWidth: 440, padding: '20px' }}>
        <GlassPanel>
          <Box component="form" onSubmit={adminLogin}>
          <Stack gap="xl">
            <Group gap="sm" justify="center">
              <ThemeIcon size={48} radius="xl" variant="gradient" gradient={{ from: '#F72585', to: '#7209B7' }}>
                <Shield size={24} />
              </ThemeIcon>
            </Group>
            
            <Box style={{ textAlign: 'center' }}>
              <Title order={1} style={{ color: 'white', textShadow: '0 0 20px rgba(247,37,133,0.5)', marginBottom: 8 }}>深海控制台</Title>
              <Text size="sm" style={{ color: 'rgba(255,255,255,0.7)' }}>管理员核心管理入口</Text>
            </Box>

            <TextInput 
              placeholder="管理员账号" 
              aria-label="管理员账号"
              autoComplete="username"
              required
              size="lg" 
              value={username} 
              onChange={(e) => setUsername(e.currentTarget.value)} 
              styles={{
                input: { background: 'rgba(0,0,0,0.2)', border: '1px solid rgba(255,255,255,0.1)', color: 'white' }
              }}
            />
            <PasswordInput 
              placeholder="管理员密码" 
              aria-label="管理员密码"
              autoComplete="current-password"
              required
              size="lg" 
              value={password} 
              onChange={(e) => setPassword(e.currentTarget.value)} 
              styles={{
                input: { background: 'rgba(0,0,0,0.2)', border: '1px solid rgba(255,255,255,0.1)', color: 'white' }
              }}
            />
            
            <Button 
              type="submit"
              size="xl" 
              leftSection={<Shield size={18} />}
              variant="gradient"
              gradient={{ from: '#7209B7', to: '#3A0CA3' }}
              loading={submitting}
              style={{
                boxShadow: '0 4px 15px rgba(114, 9, 183, 0.4)',
                border: '1px solid rgba(247,37,133,0.3)',
              }}
            >
              登入系统
            </Button>
            <ErrorText>{err}</ErrorText>
          </Stack>
          </Box>
        </GlassPanel>
      </motion.div>
    </OceanLayout>
  )
}

export function Forbidden({ onLogout }: { onLogout: () => void }) {
  return (
    <OceanLayout>
      <motion.div className="loginPanelFrame" initial="hidden" animate="visible" variants={cardVariants} style={{ width: '100%', maxWidth: 440, padding: '20px' }}>
        <GlassPanel>
          <Stack gap="xl" align="center">
            <ThemeIcon size={64} radius="xl" variant="gradient" gradient={{ from: '#E63946', to: '#800F2F' }}>
              <Shield size={32} />
            </ThemeIcon>
            
            <Box style={{ textAlign: 'center' }}>
              <Title order={1} style={{ color: 'white', textShadow: '0 0 20px rgba(230,57,70,0.5)', marginBottom: 8 }}>访问受限</Title>
              <Text size="sm" style={{ color: 'rgba(255,255,255,0.7)' }}>当前账号不具备进入深海控制台的权限。</Text>
            </Box>

            <Group>
              <Button 
                onClick={onLogout} 
                size="lg" 
                leftSection={<LogOut size={18} />}
                variant="outline"
                color="rgba(255,255,255,0.5)"
              >
                退出登录
              </Button>
              <Anchor href="/" style={{ color: '#48CAE4' }}>返回账号页</Anchor>
            </Group>
          </Stack>
        </GlassPanel>
      </motion.div>
    </OceanLayout>
  )
}
