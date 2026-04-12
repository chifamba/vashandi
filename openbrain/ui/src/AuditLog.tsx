import { useEffect, useState } from 'react';
import { api, AuditEntry } from './api';

interface Props {
  namespaceId: string;
}

export default function AuditLog({ namespaceId }: Props) {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  async function load() {
    setLoading(true);
    setError('');
    try {
      const data = await api.auditLog({ namespaceId: namespaceId || undefined, limit: 100 });
      setEntries((data ?? []).slice().reverse());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load audit log');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(); }, [namespaceId]);

  return (
    <div>
      <div className="section-header">
        <h2>Audit Log</h2>
        <button onClick={load} className="btn-secondary" disabled={loading}>Refresh</button>
      </div>

      {error && <p className="error-msg">{error}</p>}
      {loading && <p className="muted">Loading…</p>}
      {!loading && entries.length === 0 && !error && (
        <p className="muted">No audit entries found.</p>
      )}

      {entries.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Timestamp</th>
              <th>Actor Kind</th>
              <th>Action</th>
              <th>Entity Type</th>
              <th>Entity ID</th>
              <th>Chain Hash</th>
            </tr>
          </thead>
          <tbody>
            {entries.map(e => (
              <tr key={e.id}>
                <td className="nowrap">{fmtDate(e.createdAt)}</td>
                <td><span className="badge badge-type">{e.actorKind}</span></td>
                <td>{e.action}</td>
                <td>{e.entityType}</td>
                <td className="mono trunc" title={e.entityId}>{trunc(e.entityId)}</td>
                <td className="mono trunc" title={e.chainHash}>{trunc(e.chainHash)}</td>
              </tr>
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

function trunc(s: string, n = 12) {
  if (!s) return '—';
  return s.length > n ? s.slice(0, n) + '…' : s;
}
