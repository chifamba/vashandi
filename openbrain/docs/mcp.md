# OpenBrain MCP Interface

OpenBrain exposes MCP over stdio and over HTTP/SSE.

## Transports

- stdio: run `openbrain serve` and attach the binary directly
- HTTP message endpoint: `POST /mcp`
- HTTP/SSE discovery stream: `GET /mcp/sse`
- HTTP message fallback for SSE clients: `POST /mcp/message`

## Tools

### `memory_search`
```json
{ "query": "postgres migrations", "topK": 10, "namespaceId": "company-1", "agentId": "agent-1", "includeTypes": ["fact", "adr"] }
```

### `memory_note`
```json
{ "content": "Always run the DB migration before tests", "type": "fact", "namespaceId": "company-1", "agentId": "agent-1", "tier": 1, "metadata": { "source": "mcp" } }
```

### `memory_forget`
```json
{ "entityId": "memory-123", "namespaceId": "company-1", "agentId": "agent-1" }
```

### `memory_correct`
```json
{ "entityId": "memory-123", "correction": "Updated canonical setup steps", "namespaceId": "company-1", "agentId": "agent-1" }
```

### `memory_browse`
```json
{ "namespaceId": "company-1", "agentId": "agent-1", "entityType": "fact", "tier": 2, "limit": 25 }
```

### `context_compile`
```json
{ "taskQuery": "fix CI failure", "tokenBudget": 600, "namespaceId": "company-1", "agentId": "agent-1", "intent": "agent_preamble", "includeTypes": ["fact", "decision"] }
```

All MCP calls are audited in `memory_audit_log` via the shared service layer.
