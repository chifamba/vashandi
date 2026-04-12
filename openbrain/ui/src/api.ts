export interface DashboardMetrics {
  totalMemories: number;
  tierDistribution: Record<string, number>;
  staleMemoryRatio: number;
  proposalAcceptanceRate: number;
  knowledgeGapCount: number;
  topAccessedEntities: Array<{ id: string; title: string; accessCount: number }>;
}

export interface Memory {
  id: string;
  namespaceId: string;
  entityType: string;
  title: string;
  text: string;
  tier: number;
  version: number;
  accessCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface Proposal {
  id: string;
  namespaceId: string;
  proposalType: string;
  memoryIds: string[];
  summary: string;
  suggestedText: string;
  suggestedTier?: number;
  status: string;
  details: Record<string, unknown>;
  reviewedBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface AuditEntry {
  id: number;
  namespaceId: string;
  agentId: string;
  actorKind: string;
  action: string;
  entityId: string;
  entityType: string;
  chainHash: string;
  createdAt: string;
}

export interface Agent {
  id: string;
  name: string;
  trustTier: number;
  active: boolean;
  registeredAt: string;
}

function getBase(): string {
  return sessionStorage.getItem('ob_base_url') ?? 'http://localhost:3101';
}

function getHeaders(): HeadersInit {
  const token = sessionStorage.getItem('ob_token') ?? '';
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${token}`,
  };
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${getBase()}${path}`, {
    ...options,
    headers: { ...getHeaders(), ...(options?.headers ?? {}) },
  });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

export const api = {
  health: () => request<{ status: string }>('/api/v1/health'),

  dashboard: () => request<DashboardMetrics>('/api/v1/admin/dashboard'),

  daydream: (namespaceId: string) =>
    request<unknown>('/api/v1/admin/daydream', {
      method: 'POST',
      body: JSON.stringify({ namespaceId }),
    }),

  memories: (params: { namespaceId?: string; entityType?: string; tier?: string; limit?: number }) => {
    const q = new URLSearchParams();
    if (params.namespaceId) q.set('namespaceId', params.namespaceId);
    if (params.entityType && params.entityType !== 'all') q.set('entityType', params.entityType);
    if (params.tier !== undefined && params.tier !== 'all') q.set('tier', params.tier);
    q.set('limit', String(params.limit ?? 50));
    return request<Memory[]>(`/api/v1/memories?${q}`);
  },

  searchMemories: async (namespaceId: string, query: string, topK = 20): Promise<Memory[]> => {
    const data = await request<{ records: Memory[] }>('/api/v1/memories/search', {
      method: 'POST',
      body: JSON.stringify({ namespaceId, query, topK }),
    });
    return data?.records ?? [];
  },

  proposals: (params: { namespaceId?: string; status?: string }) => {
    const q = new URLSearchParams();
    if (params.namespaceId) q.set('namespaceId', params.namespaceId);
    if (params.status && params.status !== 'all') q.set('status', params.status);
    return request<Proposal[]>(`/api/v1/admin/proposals?${q}`);
  },

  resolveProposal: (namespaceId: string, proposalId: string, action: 'approved' | 'rejected') =>
    request<unknown>(`/api/v1/namespaces/${namespaceId}/proposals/${proposalId}/resolve`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    }),

  auditLog: (params: { namespaceId?: string; limit?: number }) => {
    const q = new URLSearchParams();
    if (params.namespaceId) q.set('namespaceId', params.namespaceId);
    q.set('limit', String(params.limit ?? 50));
    return request<AuditEntry[]>(`/api/v1/audit/log?${q}`);
  },

  agents: (namespaceId?: string) => {
    const q = new URLSearchParams();
    if (namespaceId) q.set('namespaceId', namespaceId);
    return request<Agent[]>(`/api/v1/agents?${q}`);
  },
};
