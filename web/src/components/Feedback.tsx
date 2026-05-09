import type { ReactNode } from 'react'
import { Alert, Center, Loader, Text } from '@mantine/core'

export function LoadingScreen() {
  return <Center h="100vh" className="loadingScreen"><Loader size="sm" />加载中</Center>
}

export function EmptyState({ children }: { children: ReactNode }) {
  return <Text className="emptyState">{children}</Text>
}

export function ErrorText({ children }: { children?: ReactNode }) {
  if (!children) return null
  return <Alert color="red" variant="light" mt="md">{children}</Alert>
}
