import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { dataworksApi } from '@/api/dataworks'
import { productFixture } from '@/test/dataworks-fixtures'
import { useAuthStore } from '@/stores/auth-store'
import type { DataProduct } from '@/types/dataworks'
import { ProductsPage } from './products-page'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.setState({ mode: 'checking', user: null })
})

function renderPage(products: DataProduct[]) {
  vi.spyOn(dataworksApi, 'products').mockResolvedValue({ products })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><MemoryRouter><ProductsPage /></MemoryRouter></QueryClientProvider>)
}

describe('ProductsPage management controls', () => {
  it('shows create/edit for writers and only hard-deletes draft or archived products', async () => {
    useAuthStore.setState({ mode: 'legacy', user: null })
    const draft = { ...productFixture, id: 'draft-id', product_key: 'draft-product', name_ko: '초안 상품', status: 'draft' as const }
    const published = { ...productFixture, id: 'published-id', product_key: 'published-product', name_ko: '출시 상품', status: 'published' as const }
    renderPage([draft, published])

    await screen.findByText('초안 상품')
    expect(screen.getByRole('button', { name: '상품 등록' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '초안 상품 수정' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '초안 상품 삭제' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '출시 상품 삭제' })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: '상품 등록' }))
    expect(screen.getByRole('dialog', { name: '새 데이터 상품 등록' })).toBeInTheDocument()
  })

  it('keeps catalog browsing read-only without admin:write', async () => {
    useAuthStore.setState({ mode: 'jwt', user: { scopes: ['admin:read'] } })
    const draft = { ...productFixture, id: 'draft-id', product_key: 'draft-product', name_ko: '조회 전용 상품', status: 'draft' as const }
    renderPage([draft])

    await screen.findByText('조회 전용 상품')
    expect(screen.queryByRole('button', { name: '상품 등록' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '조회 전용 상품 수정' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '조회 전용 상품 삭제' })).not.toBeInTheDocument()
  })
})
