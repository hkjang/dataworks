import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router-dom'

import { AppProviders } from '@/app/providers'
import { router } from '@/app/router'
import { AuthBoundary } from '@/features/auth/auth-boundary'
import '@/index.css'

const root = document.getElementById('root')
if (!root) throw new Error('Data Works root element was not found')

createRoot(root).render(
  <StrictMode>
    <AppProviders>
      <AuthBoundary>
        <RouterProvider router={router} />
      </AuthBoundary>
    </AppProviders>
  </StrictMode>,
)
