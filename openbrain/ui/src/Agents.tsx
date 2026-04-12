import { useCallback, useEffect, useState } from 'react';
import { api, Agent } from './api';

interface Props {
  namespaceId: string;
}

export default function Agents({ namespaceId }: Props) {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const data = await api.agents(namespaceId || undefined);
      setAgents(data ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agents');
    } finally {
      setLoading(false);
    }
  }, [namespaceId]);

  useEffect(() => { load(); }, [load]);

  return (
    <div>
      <div className="section-header">
        <h2>Agents</h2>
        <button onClick={load} className="btn-secondary" disabled={loading}>Refresh</button>
      </div>

      {error && <p className="error-msg">{error}</p>}
      {loading && <p className="muted">Loading…</p>}
      {!loading && agents.length === 0 && !error && (
        <p className="muted">No agents found.</p>
      )}

      {agents.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Trust Tier</th>
              <th>Active</th>
              <th>Registered At</th>
              <th>ID</th>
            </tr>
          </thead>
          <tbody>
            {agents.map(a => (
              <tr key={a.id}>
                <td>{a.name}</td>
                <td><span className={`badge badge-tier tier-${a.trustTier}`}>L{a.trustTier}</span></td>
                <td>
                  <span className={`badge ${a.active ? 'badge-active' : 'badge-inactive'}`}>
                    {a.active ? 'Active' : 'Inactive'}
                  </span>
                </td>
                <td>{fmtDate(a.registeredAt)}</td>
                <td className="mono trunc" title={a.id}>{a.id}</td>
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
