import { Card, Button, ActionIcon, TextInput, Textarea, createTheme, rem } from '@mantine/core'

export const monetBlue = [
  '#F0F4FF',
  '#E0E9FF',
  '#C2D6FF',
  '#99B8FF',
  '#6690FF',
  '#3366FF',
  '#254EDB',
  '#1A36B8',
  '#10218A',
  '#0A145C',
] as const

export const monetTheme = createTheme({
  colors: {
    brandBlue: monetBlue,
  },
  primaryColor: 'brandBlue',
  primaryShade: 5,
  fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  fontFamilyMonospace: '"Geist Mono", "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
  headings: {
    fontFamily: 'Geist, Inter, ui-sans-serif, system-ui, sans-serif',
    fontWeight: '600',
    sizes: {
      h1: { fontSize: rem(24), lineHeight: '1.2', fontWeight: '600' },
      h2: { fontSize: rem(16), lineHeight: '1.35', fontWeight: '500' },
      h3: { fontSize: rem(14), lineHeight: '1.4', fontWeight: '500' },
    },
  },
  spacing: {
    xs: rem(8),
    sm: rem(16),
    md: rem(24),
    lg: rem(32),
    xl: rem(40),
  },
  radius: {
    xs: rem(4),
    sm: rem(6),
    md: rem(8),
    lg: rem(16),
    xl: rem(24),
  },
  defaultRadius: 'md',
  shadows: {
    xs: '0 1px 2px rgba(10, 20, 92, 0.05)',
    sm: '0 4px 20px -2px rgba(51, 102, 255, 0.04), 0 0 1px rgba(10, 20, 92, 0.06)',
    md: '0 10px 30px -4px rgba(51, 102, 255, 0.08)',
    lg: '0 12px 24px -8px rgba(51, 102, 255, 0.15)',
    xl: '0 24px 60px -16px rgba(51, 102, 255, 0.18)',
  },
  fontSizes: {
    xs: rem(12),
    sm: rem(14),
    md: rem(16),
    lg: rem(24),
    xl: rem(36),
  },
  lineHeights: {
    xs: '1.25',
    sm: '1.4',
    md: '1.5',
    lg: '1.2',
    xl: '1.1',
  },
  components: {
    Button: Button.extend({
      defaultProps: {
        radius: 'md',
        fw: 500,
      },
    }),
    ActionIcon: ActionIcon.extend({
      defaultProps: {
        radius: 'md',
        variant: 'subtle',
      },
    }),
    Card: Card.extend({
      defaultProps: {
        radius: 'lg',
        p: 'md',
        shadow: 'sm',
        withBorder: true,
      },
    }),
    TextInput: TextInput.extend({
      defaultProps: {
        radius: 'md',
        size: 'sm',
      },
    }),
    Textarea: Textarea.extend({
      defaultProps: {
        radius: 'md',
        size: 'sm',
      },
    }),
  },
})
