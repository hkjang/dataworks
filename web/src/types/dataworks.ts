export type ProductStatus =
  | 'draft'
  | 'review'
  | 'risk_review'
  | 'approved'
  | 'published'
  | 'archived'

export interface DataAsset {
  id: string
  asset_key: string
  name: string
  domain: string
  owner: string
  columns_summary: string
  sensitivity: string
  refresh_cycle: string
  created_at: string
  updated_at: string
}

export interface AssetReadiness {
  asset_key: string
  schema_score: number
  freshness_score: number
  sample_score: number
  missingness_score: number
  sensitivity_score: number
  external_sharing_score: number
  api_readiness_score: number
  billing_readiness_score: number
  overall_score: number
  notes: string
  updated_by: string
  updated_at: string
}

export interface DataProduct {
  id: string
  product_key: string
  name_ko: string
  name_en: string
  short_name: string
  description: string
  executive_summary: string
  sales_pitch: string
  source_type: string
  source_ref: string
  owner: string
  allowed_teams: string[]
  sensitivity: string
  status: ProductStatus
  version: number
  target_industries: string[]
  target_customers: string[]
  pricing_model: string
  api_spec: string
  poc_plan: string
  risk_score: number
  revenue_score: number
  differentiation: string
  similar_products: string[]
  updated_by: string
  created_at: string
  updated_at: string
}

export interface HomeDashboard {
  total_assets: number
  total_products: number
  published_products: number
  review_pending: number
  high_risk: number
  poc_pending: number
  ideas_total: number
  avg_revenue_score: number
}

export interface TopProduct {
  product_key: string
  name: string
  revenue_score: number
  risk_score: number
  status: ProductStatus
}

export interface ActionSummary {
  approval_pending: number
  blocked_launches: number
  low_fit_scores: number
  expiring_contracts: number
  inactive_access: number
  stale_watermarks: number
  negative_margin: number
  retirement_candidates: number
}

export interface ActionItem {
  type: string
  severity: 'low' | 'medium' | 'high' | string
  product_key?: string
  title?: string
  next_action?: string
  blocked_reasons?: string[]
  missing_approvals?: string[]
  customer_segment?: string
  fit_score?: number
  contract_key?: string
  customer_key?: string
  valid_to?: string
  asset_key?: string
  delay_status?: string
  estimated_margin?: number
  recommendation?: string
  reason?: string
}

export interface ProductCanvas {
  product_key: string
  customer_problem: string
  buyer: string
  use_cases: string
  provided_data: string
  differentiation: string
  pricing_model: string
  risk_notes: string
  poc_success_criteria: string
  expected_revenue: string
  owner: string
  updated_by: string
  created_at: string
  updated_at: string
}

export interface ApprovalTrace {
  id: string
  product_key: string
  step: string
  status: 'pending' | 'approved' | 'rejected' | 'waived' | 'expired' | string
  required: boolean
  evidence_ref: string
  notes: string
  decided_by: string
  expires_at: string
  created_at: string
  updated_at: string
}

export interface EvidencePack {
  product_key: string
  pack_json: string
  artifact_ref: string
  created_by: string
  created_at: string
  updated_at: string
}

export interface PublishGate {
  product_key: string
  strict_gate: boolean
  allowed: boolean
  minimum_readiness: number
  required_approvals: string[]
  approval_status: Record<string, string>
  missing_approvals: string[]
  missing_evidence: string[]
  blocked_reasons: string[]
  warnings: string[]
  asset_readiness: AssetReadiness[]
  checked_at: string
  metadata?: Record<string, unknown>
  quality_passed: boolean
  risk_reviewed: boolean
  api_contract_configured: boolean
  sla_configured: boolean
  pricing_model_configured: boolean
  masking_configured: boolean
}

export interface ContractVersion {
  id: string
  product_key: string
  version: number
  contract_json: string
  status: string
  created_by: string
  created_at: string
}

export interface FactoryRun {
  id: string
  run_type: string
  model: string
  prompt_version: string
  input_hash: string
  output_ref: string
  parent_run_id: string
  policy_decision: string
  token_cost: number
  status: string
  latency_ms: number
  created_by: string
  created_at: string
}

export interface PortfolioGraphNode {
  id: string
  type: string
  label: string
  status?: string
  domain?: string
  sensitivity?: string
  risk_score?: number
  revenue_score?: number
  customer_type?: string
  success?: boolean
}

export interface PortfolioGraphEdge {
  from: string
  to: string
  relation_type?: string
  weight?: number
}

export interface PortfolioGraph {
  nodes: PortfolioGraphNode[]
  edges: PortfolioGraphEdge[]
  relationships?: unknown[]
}
