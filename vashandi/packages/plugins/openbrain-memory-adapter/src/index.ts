// These types should eventually move to @paperclipai/shared or a dedicated types package
export interface MemoryAdapterCapabilities {
  profile?: boolean;
  browse?: boolean;
  correction?: boolean;
  asyncIngestion?: boolean;
  multimodal?: boolean;
  providerManagedExtraction?: boolean;
}

export interface MemoryScope {
  companyId: string;
  agentId?: string;
  projectId?: string;
  issueId?: string;
  runId?: string;
  subjectId?: string;
}

export interface MemorySourceRef {
  kind: string;
  sourceId: string;
}

export interface MemoryUsage {
  provider: string;
  model?: string;
  inputTokens?: number;
  outputTokens?: number;
  embeddingTokens?: number;
  costCents?: number;
}

export interface MemoryWriteRequest {
  bindingKey: string;
  scope: MemoryScope;
  source: MemorySourceRef;
  content: string;
  metadata?: Record<string, unknown>;
  mode?: "append" | "upsert" | "summarize";
}

export interface MemoryRecordHandle {
  providerKey: string;
  providerRecordId: string;
}

export interface MemoryQueryRequest {
  bindingKey: string;
  scope: MemoryScope;
  query: string;
  topK?: number;
  intent?: "agent_preamble" | "answer" | "browse";
  metadataFilter?: Record<string, unknown>;
}

export interface MemorySnippet {
  handle: MemoryRecordHandle;
  text: string;
  score?: number;
  summary?: string;
  source?: MemorySourceRef;
  metadata?: Record<string, unknown>;
}

export interface MemoryContextBundle {
  snippets: MemorySnippet[];
  profileSummary?: string;
  usage?: MemoryUsage[];
}

export interface MemoryAdapter {
  key: string;
  capabilities: MemoryAdapterCapabilities;
  write(req: MemoryWriteRequest): Promise<{
    records?: MemoryRecordHandle[];
    usage?: MemoryUsage[];
  }>;
  query(req: MemoryQueryRequest): Promise<MemoryContextBundle>;
  get(handle: MemoryRecordHandle, scope: MemoryScope): Promise<MemorySnippet | null>;
  forget(handles: MemoryRecordHandle[], scope: MemoryScope): Promise<{ usage?: MemoryUsage[] }>;
}

export class OpenBrainMemoryAdapter implements MemoryAdapter {
  public key = "openbrain";
  public capabilities: MemoryAdapterCapabilities = {
    asyncIngestion: true,
  };

  private baseUrl = process.env.OPENBRAIN_URL || "http://localhost:8080";
  private apiKey = process.env.OPENBRAIN_API_KEY || "dev_secret_token";

  private getHeaders() {
    return {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${this.apiKey}`,
    };
  }

  async write(req: MemoryWriteRequest): Promise<{ records?: MemoryRecordHandle[]; usage?: MemoryUsage[] }> {
    const url = `${this.baseUrl}/api/v1/namespaces/${req.scope.companyId}/memories/ingest`;

    // Convert Paperclip request to OpenBrain format
    const body = {
      records: [
        {
          id: req.source.sourceId, // Simple mapping for now
          text: req.content,
          metadata: req.metadata || {}
        }
      ]
    };

    try {
      const response = await fetch(url, {
        method: "POST",
        headers: this.getHeaders(),
        body: JSON.stringify(body)
      });

      if (!response.ok) {
        console.warn(`OpenBrain ingest failed: ${response.statusText}. Falling back.`);
        return { records: [] };
      }

      return {
        records: [{ providerKey: this.key, providerRecordId: req.source.sourceId }],
        usage: [{ provider: "openbrain", costCents: 1 }] // Synthetic cost for task 1.4
      };
    } catch (err) {
      console.warn("Failed to reach OpenBrain memory service:", err);
      return { records: [] };
    }
  }

  async query(req: MemoryQueryRequest): Promise<MemoryContextBundle> {
    const url = `${this.baseUrl}/api/v1/namespaces/${req.scope.companyId}/memories/query`;

    const body = {
      query: req.query,
      limit: req.topK || 10
    };

    try {
      const response = await fetch(url, {
        method: "POST",
        headers: this.getHeaders(),
        body: JSON.stringify(body)
      });

      if (!response.ok) {
        console.warn(`OpenBrain query failed: ${response.statusText}. Falling back.`);
        return { snippets: [], usage: [] };
      }

      const data = await response.json();
      const snippets = (data.records || []).map((r: any) => ({
        handle: { providerKey: this.key, providerRecordId: r.id },
        text: r.text,
        metadata: r.metadata
      }));

      return {
        snippets,
        usage: [{ provider: "openbrain", costCents: 1 }] // Synthetic cost for task 1.4
      };
    } catch (err) {
      console.warn("Failed to reach OpenBrain memory service for query:", err);
      return { snippets: [], usage: [] };
    }
  }

  async get(handle: MemoryRecordHandle, scope: MemoryScope): Promise<MemorySnippet | null> {
    // OpenBrain V1 doesn't have a direct get-by-id yet, fallback to query for now
    try {
      const bundle = await this.query({
        bindingKey: "openbrain",
        scope: scope,
        query: "", // Or some ID specific query
      });
      return bundle.snippets.find(s => s.handle.providerRecordId === handle.providerRecordId) || null;
    } catch (err) {
      console.warn("Failed to get specific memory from OpenBrain:", err);
      return null;
    }
  }

  async forget(handles: MemoryRecordHandle[], scope: MemoryScope): Promise<{ usage?: MemoryUsage[] }> {
    const url = `${this.baseUrl}/api/v1/namespaces/${scope.companyId}/memories/forget`;

    const body = {
      record_ids: handles.map(h => h.providerRecordId)
    };

    try {
      const response = await fetch(url, {
        method: "POST",
        headers: this.getHeaders(),
        body: JSON.stringify(body)
      });

      if (!response.ok) {
        console.warn(`OpenBrain forget failed: ${response.statusText}. Falling back.`);
        return {};
      }

      return {};
    } catch (err) {
      console.warn("Failed to reach OpenBrain memory service for forget operation:", err);
      return {};
    }
  }
}
