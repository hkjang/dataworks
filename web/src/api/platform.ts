import { apiRequest } from './client'

export interface UsageTotals {
  requests: number
  tokens: number
  cost_krw: number
  errors: number
}

export interface PersonalProfile {
  requests: number
  total_cost_krw: number
  avg_cost_per_request: number
  avg_latency_ms: number
  success_rate: number
  error_rate: number
  cache_rate: number
  text2sql_usage_rate: number
  mcp_usage_rate: number
  risk_score: number
  summary: string
}

export interface PersonalDashboard {
  user_id: string
  today: UsageTotals
  month: UsageTotals
  profile: PersonalProfile
  potential_savings_krw: number
  potential_savings_model: string
  key_alerts: Array<{ id?: string; name?: string; severity?: string; flags?: string[] }>
  recent_failures: Array<{ id: string; model: string; status_code: number; error: string; created_at: string }>
}

export interface PersonalAPIKey {
  id: string
  name: string
  role: string
  status: string
  scopes: string[]
  allowed_ips: string[]
  allowed_models: string[]
  denied_models: string[]
  allowed_providers: string[]
  denied_providers: string[]
  budget_limit_krw: number
  expires_at: string
  created_at: string
}

export interface APIKeyPolicyInput {
  scopes?: string[]
  allowed_ips?: string[]
  allowed_models?: string[]
  denied_models?: string[]
  allowed_providers?: string[]
  denied_providers?: string[]
  budget_limit_krw?: number
  expires_at?: string
}

export interface MyKeysResponse {
  api_keys: PersonalAPIKey[]
  role: string
  grantable_scopes: string[]
}

export interface KeycloakConfig {
  enabled: boolean
  issuer_url: string
  client_id: string
  client_secret_set: boolean
  redirect_uri: string
  scopes: string[]
  default_role: string
  role_claim: string
  group_claim: string
  allow_local_login: boolean
  auto_login: boolean
  role_map: Record<string, string>
  source: string
  updated_at: string
}

export interface RuntimeSetting {
  key: string
  category: string
  type: 'string' | 'int' | 'bool' | 'float' | 'duration' | 'csv'
  is_secret: boolean
  is_set?: boolean
  value: string
  source: string
  effective_source: string
  restart_required: boolean
  read_only: boolean
  description: string
  permission_group: string
  can_write: boolean
}

export interface TrackingStatus {
  enabled: boolean
  provider: string
  providers: string[]
  placement: string
  include_admin: boolean
  active: boolean
  active_admin: boolean
  problem: string
  momento_proxy: boolean
  proxy_path: string
  proxy_target: string
  script_sources: string[]
  connect_sources: string[]
  image_sources: string[]
  report_path: string
  snippet: string
}

export interface TrackingViolation {
  origin: string
  directive: string
  page: string
  count: number
  first_seen: string
  last_seen: string
  allowed: boolean
}

export interface ProviderConfig {
  name: string
  base_url: string
  api_key_configured: boolean
  timeout_ms: number
  enabled: boolean
  model_patterns: string
  created_at: string
}

export interface RoleInfo {
  role: string
  scopes: string[]
  default_home: string
  is_admin: boolean
  is_system: boolean
  rank: number
  description: string
  user_count: number
  active_user_count: number
  can_assign: boolean
}

export interface RoleCatalogResponse {
  roles: RoleInfo[]
  all_scopes: string[]
}

export interface AdminUserSummary {
  id: string
  email: string
  name: string
  role: string
  status: string
  team_id: string
  created_at: string
}

export const platformApi = {
  personalDashboard: () => apiRequest<PersonalDashboard>('/me/dashboard'),
  myKeys: () => apiRequest<MyKeysResponse>('/me/keys'),
  createMyKey: (payload: { name: string } & APIKeyPolicyInput) =>
    apiRequest<{ api_key: PersonalAPIKey; secret: string }>('/me/keys', { method: 'POST', body: JSON.stringify(payload) }),
  updateMyKeyPolicy: (id: string, policy: APIKeyPolicyInput) =>
    apiRequest<PersonalAPIKey>(`/me/keys/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(policy) }),
  rotateMyKey: (id: string) =>
    apiRequest<{ api_key: PersonalAPIKey; secret: string; rotated_from: string }>(`/me/keys/${encodeURIComponent(id)}/rotate`, { method: 'POST', body: '{}' }),
  revokeMyKey: (id: string) => apiRequest(`/me/keys/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  keycloakConfig: () => apiRequest<KeycloakConfig>('/admin/sso/keycloak/config'),
  saveKeycloakConfig: (payload: Record<string, unknown>) =>
    apiRequest<void>('/admin/sso/keycloak/config', { method: 'PUT', body: JSON.stringify(payload) }),
  testKeycloak: () => apiRequest<{ ok: boolean; reason?: string; stage?: string; issuer?: string; rsa_signing_keys?: number }>('/admin/sso/keycloak/test', { method: 'POST' }),
  settings: () => apiRequest<{ settings: RuntimeSetting[] }>('/admin/settings/effective'),
  saveSetting: (key: string, value: string) =>
    apiRequest(`/admin/settings/by-key/${encodeURIComponent(key)}`, { method: 'PUT', body: JSON.stringify({ value, reason: 'React 관리자 설정' }) }),
  revertSetting: (key: string) => apiRequest(`/admin/settings/by-key/${encodeURIComponent(key)}`, { method: 'DELETE' }),
  trackingStatus: () => apiRequest<TrackingStatus>('/admin/tracking/status'),
  trackingViolations: () => apiRequest<{ items: TrackingViolation[] }>('/admin/tracking/violations'),
  clearTrackingViolations: () => apiRequest<void>('/admin/tracking/violations', { method: 'DELETE' }),
  allowTrackingOrigin: (origin: string) =>
    apiRequest<{ key: string; value: string; items: TrackingViolation[] }>('/admin/tracking/violations/allow', { method: 'POST', body: JSON.stringify({ origin }) }),
  providers: () => apiRequest<{ providers: ProviderConfig[] }>('/admin/providers'),
  saveProvider: (payload: Record<string, unknown>) =>
    apiRequest<{ provider: ProviderConfig }>('/admin/providers', { method: 'POST', body: JSON.stringify(payload) }),
  roles: () => apiRequest<RoleCatalogResponse>('/admin/roles'),
  saveRole: (payload: { role: string; description: string; scopes: string[]; default_home: string }) =>
    apiRequest<{ role: RoleInfo }>('/admin/roles', { method: 'POST', body: JSON.stringify(payload) }),
  deleteRole: (role: string) =>
    apiRequest<{ role: string; deleted: boolean }>(`/admin/roles?role=${encodeURIComponent(role)}`, { method: 'DELETE' }),
  adminUsers: () => apiRequest<{ auth_users: AdminUserSummary[] }>('/admin/users'),
  updateAdminUser: (id: string, payload: { role?: string; status?: 'active' | 'disabled' }) =>
    apiRequest<{ user: AdminUserSummary; team_id: string }>(`/admin/users/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(payload) }),
}
