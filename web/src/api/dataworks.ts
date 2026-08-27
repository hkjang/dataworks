import { ApiError, apiRequest } from './client'
import type {
  ActionItem,
  ActionSummary,
  ApprovalTrace,
  AssetReadiness,
  ContractVersion,
  DataAsset,
  DataProduct,
  EvidencePack,
  FactoryRun,
  HomeDashboard,
  PortfolioGraph,
  ProductCanvas,
  PublishGate,
  TopProduct,
} from '@/types/dataworks'

const root = '/admin/dataworks'

export const dataworksApi = {
  home: async () => {
    const response = await apiRequest<{ dashboard: HomeDashboard; top_products: TopProduct[] | null }>(`${root}/home`)
    return { ...response, top_products: response.top_products ?? [] }
  },
  actionCenter: async () => {
    const response = await apiRequest<{ summary: ActionSummary; actions: ActionItem[] | null }>(`${root}/action-center`)
    return { ...response, actions: response.actions ?? [] }
  },
  assets: async () => {
    const response = await apiRequest<{ assets: DataAsset[] | null }>(`${root}/assets`)
    return { ...response, assets: response.assets ?? [] }
  },
  readiness: async (assetKey = '') => {
    const response = await apiRequest<{ readiness: AssetReadiness[] | null }>(
      `${root}/assets/readiness${assetKey ? `?asset_key=${encodeURIComponent(assetKey)}` : ''}`,
    )
    return { ...response, readiness: response.readiness ?? [] }
  },
  products: async () => {
    const response = await apiRequest<{ products: DataProduct[] | null }>(`${root}/products`)
    return { ...response, products: response.products ?? [] }
  },
  canvas: (productKey: string) =>
    apiRequest<{ canvas: ProductCanvas; draft: boolean }>(
      `${root}/products/${encodeURIComponent(productKey)}/canvas`,
    ),
  approvals: async (productKey: string) => {
    const response = await apiRequest<{ approvals: ApprovalTrace[] | null }>(
      `${root}/products/${encodeURIComponent(productKey)}/approvals`,
    )
    return { ...response, approvals: response.approvals ?? [] }
  },
  evidencePack: (productKey: string) =>
    apiRequest<{ evidence_pack: EvidencePack }>(
      `${root}/products/${encodeURIComponent(productKey)}/evidence-pack`,
    ).catch((error: unknown) => {
      if (error instanceof ApiError && error.status === 404) return { evidence_pack: null }
      throw error
    }) as Promise<{ evidence_pack: EvidencePack | null }>,
  publishGate: async (productKey: string) => {
    const response = await apiRequest<{ publish_gate: PublishGate }>(
      `${root}/products/${encodeURIComponent(productKey)}/publish-gate`,
    )
    return {
      ...response,
      publish_gate: {
        ...response.publish_gate,
        required_approvals: response.publish_gate.required_approvals ?? [],
        approval_status: response.publish_gate.approval_status ?? {},
        missing_approvals: response.publish_gate.missing_approvals ?? [],
        missing_evidence: response.publish_gate.missing_evidence ?? [],
        blocked_reasons: response.publish_gate.blocked_reasons ?? [],
        warnings: response.publish_gate.warnings ?? [],
        asset_readiness: response.publish_gate.asset_readiness ?? [],
      },
    }
  },
  contractVersions: (productKey: string) =>
    apiRequest<{ contract_version: ContractVersion | null }>(
      `${root}/products/${encodeURIComponent(productKey)}/contract-versions`,
    ),
  publish: (productKey: string) =>
    apiRequest<{ ok: boolean; status: string }>(
      `${root}/products/${encodeURIComponent(productKey)}/publish`,
      { method: 'POST' },
    ),
  factoryRuns: async () => {
    const response = await apiRequest<{ runs: FactoryRun[] | null; evaluation_summaries: Record<string, unknown> | null }>(
      `${root}/factory/runs?days=30&limit=100`,
    )
    return { ...response, runs: response.runs ?? [], evaluation_summaries: response.evaluation_summaries ?? {} }
  },
  portfolioGraph: async () => {
    const response = await apiRequest<{ graph: PortfolioGraph }>(`${root}/portfolio/graph`)
    return {
      ...response,
      graph: {
        ...response.graph,
        nodes: response.graph?.nodes ?? [],
        edges: response.graph?.edges ?? [],
        relationships: response.graph?.relationships ?? [],
      },
    }
  },
}
