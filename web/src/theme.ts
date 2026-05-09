import {
  ActionIcon,
  Badge,
  Button,
  Card,
  NumberInput,
  PasswordInput,
  Select,
  TextInput,
  Textarea,
  createTheme,
  rem,
} from '@mantine/core'

export const oceanicBlue = [
  '#F4F9FA', '#EBF4FA', '#D6E6F2', '#B5D0E8', '#8CB5DB', 
  '#5C96C7', '#2C74B3', '#1A518B', '#0F4C81', '#1A365D'
] as const

export const monetTheme = createTheme({
  colors: {
    brandBlue: oceanicBlue,
  },
  primaryColor: 'brandBlue',
  primaryShade: 8,
  fontFamily: 'Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
  fontFamilyMonospace: '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
  headings: {
    fontFamily: 'Inter, system-ui, sans-serif',
    fontWeight: '700',
    sizes: {
      h1: { fontSize: rem(28), lineHeight: '1.2' },
      h2: { fontSize: rem(20), lineHeight: '1.3' },
      h3: { fontSize: rem(16), lineHeight: '1.4' },
    },
  },
  spacing: { xs: rem(8), sm: rem(16), md: rem(24), lg: rem(32), xl: rem(48) },
  radius: { xs: rem(8), sm: rem(12), md: rem(16), lg: rem(24), xl: rem(32) },
  defaultRadius: 'md',
  shadows: {
    xs: '0 2px 8px -2px rgba(15, 76, 129, 0.05)',
    sm: '0 4px 24px -8px rgba(15, 76, 129, 0.08)',
    md: '0 12px 32px -8px rgba(15, 76, 129, 0.12)',
    lg: '0 20px 40px -10px rgba(15, 76, 129, 0.15)',
    xl: '0 24px 48px -12px rgba(15, 76, 129, 0.18)',
  },
  components: {
    Button: Button.extend({
      defaultProps: { radius: 'xl', fw: 600, size: 'md' },
    }),
    ActionIcon: ActionIcon.extend({
      defaultProps: { radius: 'xl', variant: 'light', size: 'lg' },
    }),
    Badge: Badge.extend({
      defaultProps: { radius: 'xl', fw: 600, px: 'sm' },
    }),
    Card: Card.extend({
      defaultProps: { radius: 'xl', p: 'xl', shadow: 'sm', withBorder: false },
    }),
    TextInput: TextInput.extend({ defaultProps: { radius: 'md', size: 'md' } }),
    Textarea: Textarea.extend({ defaultProps: { radius: 'md', size: 'md' } }),
    NumberInput: NumberInput.extend({ defaultProps: { radius: 'md', size: 'md' } }),
    PasswordInput: PasswordInput.extend({ defaultProps: { radius: 'md', size: 'md' } }),
    Select: Select.extend({ defaultProps: { radius: 'md', size: 'md' } }),
  },
})
