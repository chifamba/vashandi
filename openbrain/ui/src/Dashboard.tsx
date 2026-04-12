import { useEffect, useState } from 'react';
import { api, DashboardMetrics } from './api';

interface Props {
  namespaceId: string;
  onNamespaceChange: (ns: string) => void;
}

export default function Dashboard({ namespaceId, onNamespaceChange }: Props) {
  const [metrics, setMetrics] = useState<DashboardMetrics | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [daydreaming, setDaydreaming] = useState(false);
  const [daydreamMsg, setDaydreamMsg] = useState('');

  async function load() {
    setLoading(true);
    setError('');
    try {
      const data = await api.dashboard();
      setMetrics(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load dashboard');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(); }, []);

  async function triggerDaydream() {
    setDaydreaming(true);
    setDaydreamMsg('');
    try {
      await api.daydream(namespaceId);
      setDaydreamMsg('Daydream triggered successfully.');
    } catch (err) {
      setDaydreamMsg(err instanceof Error ? err.message : 'Failed to trigger daydream');
    } finally {
      setDaydreaming(false);
    }
  }

  return (
    <div>
      <div className="section-header">
        <h2>Dashboard</h2>
        <div className="row-gap">
          <label className="inline-label">
            Namespace&nbsp;ID
            <input
              type="text"
              value={namespaceId}
              onChange={e => onNamespaceChange(e.target.value)}
              placeholder="(optional)"
              className="inline-input"
            />
          </label>
          <button onClick={load} className="btn-secondary" disabled={loading}>
            {loading ? 'Loading…' : 'Refresh'}
          </button>
          <button onClick={triggerDaydream} className="btn-primary" disabled={daydreaming}>
            {daydreaming ? 'Triggering…' : '✦ Trigger Daydream'}
          </button>
        </div>
      </div>

      {daydreamMsg && <p className={daydreamMsg.includes('success') ? 'success-msg' : 'error-msg'}>{daydreamMsg}</p>}
      {error && <p className="error-msg">{error}</p>}

      {metrics && (
        <>
          <div className="metric-grid">
            <MetricCard label="Total Memories" value={metrics.totalMemories} />
            <MetricCard label="Knowledge Gaps" value={metrics.knowledgeGapCount} />
            <MetricCard
              label="Stale Ratio"
              value={`${(metrics.staleMemoryRatio * 100).toFixed(1)}%`}
            />
            <MetricCard
              label="Proposal Acceptance"
              value={`${(metrics.proposalAcceptanceRate * 100).toFixed(1)}%`}
            />
          </div>

          <h3>Tier Distribution</h3>
          <div className="metric-grid">
            {['0', '1', '2', '3'].map(t => (
              <MetricCard
                key={t}
                label={`L${t}`}
                value={metrics.tierDistribution[t] ?? 0}
              />
            ))}
          </div>

          {metrics.topAccessedEntities.length > 0 && (
            <>
              <h3>Top Accessed Entities</h3>
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Title</th>
                    <th>Access Count</th>
                    <th>ID</th>
                  </tr>
                </thead>
                <tbody>
                  {metrics.topAccessedEntities.map(e => (
                    <tr key={e.id}>
                      <td>{e.title}</td>
                      <td>{e.accessCount}</td>
                      <td className="mono trunc">{e.id}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </>
      )}

      {loading && !metrics && <p className="muted">Loading…</p>}
    </div>
  );
}

function MetricCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="metric-card">
      <div className="metric-value">{value}</div>
      <div className="metric-label">{label}</div>
    </div>
  );
}
