import { afterEach, describe, expect, it, vi } from 'vitest'

import { dataworksApi } from './dataworks'

afterEach(() => {
  vi.unstubAllGlobals()
  sessionStorage.clear()
})

describe('dataworksApi collection normalization', () => {
  it('빈 데이터베이스의 null 컬렉션을 빈 배열과 객체로 변환한다', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      const payload = url.includes('/factory/runs')
        ? { runs: null, evaluation_summaries: null }
        : url.includes('/publish-gate')
          ? { publish_gate: { required_approvals: null, approval_status: null, missing_approvals: null, missing_evidence: null, blocked_reasons: null, warnings: null, asset_readiness: null } }
        : { dashboard: {}, top_products: null }
      return new Response(JSON.stringify(payload), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    }))

    await expect(dataworksApi.home()).resolves.toMatchObject({ top_products: [] })
    await expect(dataworksApi.factoryRuns()).resolves.toEqual({ runs: [], evaluation_summaries: {} })
    await expect(dataworksApi.publishGate('empty-product')).resolves.toMatchObject({ publish_gate: {
      required_approvals: [],
      approval_status: {},
      missing_approvals: [],
      missing_evidence: [],
      blocked_reasons: [],
      warnings: [],
      asset_readiness: [],
    } })
  })
})

describe('dataworksApi management mutations', () => {
  it('uses the supported CUD endpoints and encodes resource identifiers', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      void input
      void init
      return new Response(JSON.stringify({ ok: true, canvas: {}, approval: {}, version: 2, readiness: {} }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })
    vi.stubGlobal('fetch', fetchMock)

    await dataworksApi.saveAsset({ asset_key: 'risk/events', name: '위험 이벤트', domain: 'risk', owner: 'owner', columns_summary: '', sensitivity: 'internal', refresh_cycle: 'daily' })
    await dataworksApi.deleteAsset('risk/events')
    await dataworksApi.checkAssetReadiness('risk/events')
    await dataworksApi.saveProduct({ product_key: 'risk score', name_ko: '위험 점수' })
    await dataworksApi.deleteProduct('risk score')
    await dataworksApi.saveCanvas('risk score', { customer_problem: '', buyer: '', use_cases: '', provided_data: '', differentiation: '', pricing_model: '', risk_notes: '', poc_success_criteria: '', expected_revenue: '', owner: '' })
    await dataworksApi.saveApproval('risk score', { step: 'legal', status: 'approved', required: true, evidence_ref: '', notes: '', decided_by: '', expires_at: '' })
    await dataworksApi.createContractVersion('risk score', { contract_json: '{}', status: 'draft' })
    await dataworksApi.transitionProduct('risk score', 'submit')

    expect(fetchMock).toHaveBeenCalledTimes(9)
    expect(fetchMock.mock.calls[0][0]).toBe('/admin/dataworks/assets')
    expect(fetchMock.mock.calls[0][1]).toEqual(expect.objectContaining({ method: 'POST' }))
    expect(fetchMock.mock.calls[1][0]).toBe('/admin/dataworks/assets?asset_key=risk%2Fevents')
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({ method: 'DELETE' }))
    expect(fetchMock.mock.calls[2][0]).toBe('/admin/dataworks/assets/risk%2Fevents/readiness/check')
    expect(fetchMock.mock.calls[3][0]).toBe('/admin/dataworks/products')
    expect(fetchMock.mock.calls[4][0]).toBe('/admin/dataworks/products?product_key=risk%20score')
    expect(fetchMock.mock.calls[5][0]).toBe('/admin/dataworks/products/risk%20score/canvas')
    expect(fetchMock.mock.calls[6][0]).toBe('/admin/dataworks/products/risk%20score/approvals')
    expect(fetchMock.mock.calls[7][0]).toBe('/admin/dataworks/products/risk%20score/contract-versions')
    expect(fetchMock.mock.calls[8][0]).toBe('/admin/dataworks/products/risk%20score/submit')
  })
})
