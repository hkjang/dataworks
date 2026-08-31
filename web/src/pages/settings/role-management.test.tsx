import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { platformApi, type RoleInfo } from '@/api/platform'
import { useAuthStore } from '@/stores/auth-store'
import { RoleManagement } from './role-management'

const roles: RoleInfo[] = [
  { role: 'developer', description: '개발자', scopes: ['models:read'], default_home: '#/factory', is_admin: false, is_system: true, rank: 2, user_count: 1, active_user_count: 1, can_assign: true },
  { role: 'viewer', description: '뷰어', scopes: ['admin:read'], default_home: '#/dataworks/home', is_admin: true, is_system: true, rank: 1, user_count: 0, active_user_count: 0, can_assign: true },
  { role: 'cost_analyst', description: '비용 분석 담당', scopes: ['models:read', 'costs:read'], default_home: '#/factory', is_admin: false, is_system: false, rank: 1, user_count: 1, active_user_count: 1, can_assign: true },
]

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.setState({ mode: 'checking', user: null })
})

function renderPage() {
  vi.spyOn(platformApi, 'roles').mockResolvedValue({ roles, all_scopes: ['models:read', 'admin:read', 'admin:write', 'costs:read'] })
  vi.spyOn(platformApi, 'adminUsers').mockResolvedValue({ auth_users: [{ id: 'usr_dev', email: 'dev@example.com', name: '개발자', role: 'developer', status: 'active', team_id: 'team_data', created_at: '2026-08-31T00:00:00Z' }] })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}><RoleManagement /></QueryClientProvider>)
}

describe('RoleManagement', () => {
  it('creates custom roles and protects assigned roles from deletion', async () => {
    useAuthStore.setState({ mode: 'jwt', user: { role: 'super_admin', scopes: ['admin:read', 'admin:write'] } })
    renderPage()

    await screen.findByText('역할 카탈로그')
    expect(screen.getByRole('button', { name: 'cost_analyst 삭제' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'developer 삭제' })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '역할 만들기' }))
    expect(screen.getByRole('dialog', { name: '사용자 정의 역할 만들기' })).toBeInTheDocument()
    expect(screen.getByText('권한 영향 미리보기')).toBeInTheDocument()
  })

  it('confirms role assignment before sending the mutation', async () => {
    useAuthStore.setState({ mode: 'jwt', user: { role: 'super_admin', scopes: ['admin:read', 'admin:write'] } })
    const update = vi.spyOn(platformApi, 'updateAdminUser').mockResolvedValue({
      user: { id: 'usr_dev', email: 'dev@example.com', name: '개발자', role: 'viewer', status: 'active', team_id: 'team_data', created_at: '2026-08-31T00:00:00Z' },
      team_id: 'team_data',
    })
    renderPage()

    const select = await screen.findByRole('combobox', { name: 'dev@example.com 새 역할' })
    await userEvent.selectOptions(select, 'viewer')
    await userEvent.click(screen.getByRole('button', { name: '적용' }))
    expect(update).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog', { name: '사용자 역할 변경' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '역할 변경' }))
    await waitFor(() => expect(update).toHaveBeenCalledWith('usr_dev', { role: 'viewer' }))
  })
})
