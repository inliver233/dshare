import React from 'react'
import { createRoot } from 'react-dom/client'
import { MantineProvider } from '@mantine/core'
import { App } from './App'
import { monetTheme } from './theme'
import '@mantine/core/styles.css'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <MantineProvider theme={monetTheme} defaultColorScheme="light">
      <App />
    </MantineProvider>
  </React.StrictMode>,
)
