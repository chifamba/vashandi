import { useEffect, useState } from 'react';
import { api, Memory } from './api';

interface Props {
  namespaceId: string;
}

const ENTITY_TYPES = ['all', 'fact', 'decision', 'task', 'constraint', 'adr', 'note'];
const TIERS = ['all', '0', '1', '2', '3'];

export default function Memories({ namespaceId }: Props) {
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [entityType, setEntityType] = useState('all');
  const [tier, setTier] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [expanded, setExpanded] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError('');
    try {
      const data = await api.memories({ namespaceId: namespaceId || undefined, entityType, tier });
      setMemories(data ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load memories');
    } finally {
      setLoading(false);
    }
  }

  async function doSearch() {
    if (!searchQuery.trim()) { load(); return; }
    setLoading(true);
    setError('');
    try {
      const data = await api.searchMemories(namespaceId, searchQuery.trim());
      setMemories(data ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Search failed');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(); }, [namespaceId, entityType, tier]);

  function toggleExpand(id: string) {
    setExpanded(prev => prev === id ? null : id);
  }

  return (
    <div>
      <div className="section-header">
        <h2>Memories</h2>
        <div className="row-gap">
          <select value={entityType} onChange={e => setEntityType(e.target.value)} className="select">
            {ENTITY_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
          </select>
          <select value={tier} onChange={e => setTier(e.target.value)} className="select">
            {TIERS.map(t => <option key={t} value={t}>{t === 'all' ? 'All Tiers' : `Tier ${t}`}</option>)}
          </select>
          <button onClick={load} className="btn-secondary" disabled={loading}>Refresh</button>
        </div>
      </div>

      <div className="search-row">
        <input
          type="text"
          value={searchQuery}
          onChange={e => setSearchQuery(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && doSearch()}
          placeholder="Search memories… (Enter)"
          className="search-input"
        />
        <button onClick={doSearch} className="btn-primary" disabled={loading}>Search</button>
        {searchQuery && (
          <button onClick={() => { setSearchQuery(''); load(); }} className="btn-secondary">Clear</button>
        )}
      </div>

      {error && <p className="error-msg">{error}</p>}
      {loading && <p className="muted">Loading…</p>}

      {!loading && memories.length === 0 && !error && (
        <p className="muted">No memories found.</p>
      )}

      {memories.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Title</th>
              <th>Type</th>
              <th>Tier</th>
              <th>Access Count</th>
              <th>Updated At</th>
            </tr>
          </thead>
          <tbody>
            {memories.map(m => (
              <>
                <tr
                  key={m.id}
                  className="clickable-row"
                  onClick={() => toggleExpand(m.id)}
                >
                  <td>{m.title}</td>
                  <td><span className="badge badge-type">{m.entityType}</span></td>
                  <td><span className={`badge badge-tier tier-${m.tier}`}>L{m.tier}</span></td>
                  <td>{m.accessCount}</td>
                  <td>{fmtDate(m.updatedAt)}</td>
                </tr>
                {expanded === m.id && (
                  <tr key={`${m.id}-exp`} className="expanded-row">
                    <td colSpan={5}>
                      <div className="expanded-content">
                        <p className="mono-small">{m.text}</p>
                        <p className="meta">ID: {m.id} · Namespace: {m.namespaceId} · Version: {m.version}</p>
                      </div>
                    </td>
                  </tr>
                )}
              </>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function fmtDate(s: string) {
  try { return new Date(s).toLocaleString(); } catch { return s; }
}
