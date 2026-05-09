import { useState } from 'react'
import { ActionIcon, Button, Code, Group } from '@mantine/core'
import { Copy } from 'lucide-react'
import { copyText } from '../utils/clipboard'

type Props = {
  value: string
  className?: string
  label?: string
}

export function CopyButton({ value, className = 'copyAction', label = '' }: Props) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const copy = async () => {
    const ok = await copyText(value)
    setState(ok ? 'copied' : 'failed')
    window.setTimeout(() => setState('idle'), 1800)
  }
  const text = state === 'copied' ? '已复制' : state === 'failed' ? '长按复制' : label

  if (label) {
    return (
      <Button className={className} variant="light" leftSection={<Copy size={16} />} onClick={copy} title={state === 'failed' ? '当前浏览器限制复制，请长按文本手动复制' : '复制'}>
        {text || '复制'}
      </Button>
    )
  }

  return (
    <ActionIcon className={className} onClick={copy} title={state === 'failed' ? '当前浏览器限制复制，请长按文本手动复制' : '复制'} aria-label="复制">
      <Copy size={18} />
    </ActionIcon>
  )
}

export function CopyBox({ value }: { value: string }) {
  return (
    <Group className="copyBox" justify="space-between" wrap="nowrap">
      <Code className="machineCode">{value}</Code>
      <CopyButton value={value} className="copyButton" label="复制" />
    </Group>
  )
}
