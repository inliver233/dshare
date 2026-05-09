import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Box, Button, Card, Group, SegmentedControl, Stack, Table, Text, TextInput, Title } from '@mantine/core'
import { Activity, ArrowLeft, CheckCircle2, Search, Trophy } from 'lucide-react'
import { motion, AnimatePresence, Variants } from 'framer-motion'
import type { RankBoard, RankPeriod, RankResponse } from '../types/api'
import { api, queryString } from '../services/api'
import { EmptyState, ErrorText } from '../components/Feedback'
import { useDebouncedValue } from '../utils/hooks'
import { formatDate, formatNumber } from '../utils/format'

const PAGE_SIZE = 50

const containerVariants: Variants = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.08 } }
}
const itemVariants: Variants = {
  hidden: { opacity: 0, y: 15 },
  show: { opacity: 1, y: 0, transition: { type: 'spring', stiffness: 300, damping: 25 } }
}

function RankSkeletonRows() {
  return (
    <Stack gap="sm">
      {Array.from({ length: 5 }).map((_, index) => <Box className="skeletonRow" key={index} />)}
    </Stack>
  )
}

function RankBadge({ rank }: { rank: number }) {
  const isTop = rank <= 3
  return <Text className={isTop ? 'rankBadge topRank' : 'rankBadge'}>#{rank}</Text>
}

export function RankPage() {
  const [board, setBoard] = useState<RankBoard>('requests')
  const [period, setPeriod] = useState<RankPeriod>('all')
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebouncedValue(search, 260)
  const [data, setData] = useState<RankResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const requestID = useRef(0)

  const load = useCallback(async (offset = 0) => {
    const id = ++requestID.current
    const nextPeriod = board === 'contributions' ? 'all' : period
    if (offset === 0) {
      setLoading(true)
    } else {
      setLoadingMore(true)
    }
    setError('')
    try {
      const payload = await api<RankResponse>('/api/rank' + queryString({
        board,
        period: nextPeriod,
        q: debouncedSearch,
        limit: PAGE_SIZE,
        offset,
      }))
      if (id !== requestID.current) return
      setData((current) => offset === 0 ? payload : {
        ...payload,
        items: [...(current?.items || []), ...payload.items],
      })
    } catch (e) {
      if (id === requestID.current) setError((e as Error).message)
    } finally {
      if (id === requestID.current) {
        setLoading(false)
        setLoadingMore(false)
      }
    }
  }, [board, period, debouncedSearch])

  useEffect(() => {
    load(0)
  }, [load])

  const boardLabel = board === 'requests' ? '调用榜' : '贡献榜'
  const subtitle = useMemo(() => {
    if (board === 'contributions') return '按有效贡献账号数量排序'
    if (period === 'day') return '按 UTC 今日调用数量排序'
    if (period === 'week') return '按 UTC 本周调用数量排序'
    return '按历史总调用数量排序'
  }, [board, period])

  return (
    <Box className="rankPage">
      <motion.div variants={containerVariants} initial="hidden" animate="show">
        <motion.header className="rankHeader" variants={itemVariants}>
          <Box>
            <Button component="a" href="/" variant="subtle" size="xs" leftSection={<ArrowLeft size={16} />} mb="sm">返回控制台</Button>
            <Group gap="xs">
              <Trophy size={22} />
              <Title order={1}>排行榜</Title>
            </Group>
            <Text c="var(--color-text-body)" size="sm" mt={4}>{subtitle}</Text>
          </Box>
          <TextInput
            leftSection={<Search size={16} />}
            value={search}
            onChange={(e) => setSearch(e.currentTarget.value)}
            placeholder="搜索 Discord ID / 名称"
            autoComplete="off"
            className="rankSearch"
          />
        </motion.header>

        <motion.div variants={itemVariants}>
          <Card mb="xl">
            <Group justify="space-between" gap="sm">
              <SegmentedControl
                value={board}
                onChange={(value) => setBoard(value as RankBoard)}
                data={[
                  { value: 'requests', label: <Group gap={6}><Activity size={16} />调用榜</Group> },
                  { value: 'contributions', label: <Group gap={6}><CheckCircle2 size={16} />贡献榜</Group> },
                ]}
              />
              {board === 'requests' && (
                <SegmentedControl
                  value={period}
                  onChange={(value) => setPeriod(value as RankPeriod)}
                  data={[
                    { value: 'all', label: '总榜' },
                    { value: 'week', label: '周榜' },
                    { value: 'day', label: '日榜' },
                  ]}
                />
              )}
            </Group>
          </Card>
        </motion.div>

        <motion.div variants={itemVariants}>
          <Card>
            <Group justify="space-between" mb="md">
              <Text fw={500}>{boardLabel}</Text>
              {data?.generated_at && <Text size="xs" c="var(--color-text-muted)">更新 {formatDate(data.generated_at)}</Text>}
            </Group>
            <ErrorText>{error}</ErrorText>
            {loading ? (
              <RankSkeletonRows />
            ) : data && data.items.length > 0 ? (
              <Stack gap="md">
                <Table className="dataTable" verticalSpacing="md">
                  <Table.Tbody>
                    <AnimatePresence mode="popLayout">
                      {data.items.map((item, index) => (
                        <motion.tr
                          key={`${item.user_id}-${item.rank}`}
                          initial={{ opacity: 0, x: -10 }}
                          animate={{ opacity: 1, x: 0 }}
                          exit={{ opacity: 0 }}
                          transition={{ delay: index * 0.02, duration: 0.2 }}
                        >
                          <Table.Td w={74}><RankBadge rank={item.rank} /></Table.Td>
                          <Table.Td>
                            <Text fw={500} size="sm" truncate>{item.display_name}</Text>
                            <Text size="xs" c="var(--color-text-muted)">{item.discord_username || item.discord_id_preview}</Text>
                          </Table.Td>
                          <Table.Td ta="right"><Text fw={700}>{formatNumber(item.value)}</Text></Table.Td>
                        </motion.tr>
                      ))}
                    </AnimatePresence>
                  </Table.Tbody>
                </Table>
                {data.has_more && (
                  <Button variant="light" disabled={loadingMore} onClick={() => load(data.next_offset || data.items.length)}>
                    {loadingMore ? '加载中' : '加载更多'}
                  </Button>
                )}
              </Stack>
            ) : (
              <EmptyState>没有匹配的排行榜数据</EmptyState>
            )}
          </Card>
        </motion.div>
      </motion.div>
    </Box>
  )
}
