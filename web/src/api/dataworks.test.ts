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
