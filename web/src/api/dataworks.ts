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
  home: () =>
    apiRequest<{ dashboard: HomeDashboard; top_products: TopProduct[] }>(`${root}/home`),
  actionCenter: () =>
    apiRequest<{ summary: ActionSummary; actions: ActionItem[] }>(`${root}/action-center`),
  assets: () => apiRequest<{ assets: DataAsset[] }>(`${root}/assets`),
  readiness: (assetKey = '') =>
    apiRequest<{ readiness: AssetReadiness[] }>(
      `${root}/assets/readiness${assetKey ? `?asset_key=${encodeURIComponent(assetKey)}` : ''}`,
    ),
  products: () => apiRequest<{ products: DataProduct[] }>(`${root}/products`),
  canvas: (productKey: string) =>
    apiRequest<{ canvas: ProductCanvas; draft: boolean }>(
      `${root}/products/${encodeURIComponent(productKey)}/canvas`,
    ),
  approvals: (productKey: string) =>
    apiRequest<{ approvals: ApprovalTrace[] }>(
      `${root}/products/${encodeURIComponent(productKey)}/approvals`,
    ),
  evidencePack: (productKey: string) =>
    apiRequest<{ evidence_pack: EvidencePack }>(
      `${root}/products/${encodeURIComponent(productKey)}/evidence-pack`,
    ).catch((error: unknown) => {
      if (error instanceof ApiError && error.status === 404) return { evidence_pack: null }
      throw error
    }) as Promise<{ evidence_pack: EvidencePack | null }>,
  publishGate: (productKey: string) =>
    apiRequest<{ publish_gate: PublishGate }>(
      `${root}/products/${encodeURIComponent(productKey)}/publish-gate`,
    ),
  contractVersions: (productKey: string) =>
    apiRequest<{ contract_version: ContractVersion | null }>(
      `${root}/products/${encodeURIComponent(productKey)}/contract-versions`,
    ),
  publish: (productKey: string) =>
    apiRequest<{ ok: boolean; status: string }>(
      `${root}/products/${encodeURIComponent(productKey)}/publish`,
      { method: 'POST' },
    ),
  factoryRuns: () =>
    apiRequest<{ runs: FactoryRun[]; evaluation_summaries: Record<string, unknown> }>(
      `${root}/factory/runs?days=30&limit=100`,
    ),
  portfolioGraph: () =>
    apiRequest<{ graph: PortfolioGraph }>(`${root}/portfolio/graph`),
}
