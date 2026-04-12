import { useEffect, useState } from 'react';
import { api, Proposal } from './api';

interface Props {
  namespaceId: string;
}

const STATUSES = ['all', 'pending', 'approved', 'rejected'];

export default function Proposals({ namespaceId }: Props) {
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [status, setStatus] = useState('pending');
  const [expanded, setExpanded] = useState<string | null>(null);
  const [resolving, setResolving] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError('');
    try {
      const data = await api.proposals({ namespaceId: namespaceId || undefined, status });
      setProposals(data ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load proposals');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { load(); }, [namespaceId, status]);

  async function resolve(p: Proposal, action: 'approved' | 'rejected') {
    setResolving(p.id);
    try {
      await api.resolveProposal(p.namespaceId, p.id, action);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to resolve proposal');
    } finally {
      setResolving(null);
    }
  }

  function toggleExpand(id: string) {
    setExpanded(prev => prev === id ? null : id);
  }

  return (
    <div>
      <div className="section-header">
        <h2>Proposals</h2>
        <div className="row-gap">
          <select value={status} onChange={e => setStatus(e.target.value)} className="select">
            {STATUSES.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
          <button onClick={load} className="btn-secondary" disabled={loading}>Refresh</button>
        </div>
      </div>

      {error && <p className="error-msg">{error}</p>}
      {loading && <p className="muted">Loading…</p>}
      {!loading && proposals.length === 0 && !error && (
        <p className="muted">No proposals found.</p>
      )}

      {proposals.length > 0 && (
        <table className="data-table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Summary</th>
              <th>Created At</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {proposals.map(p => (
              <>
                <tr
                  key={p.id}
                  className="clickable-row"
                  onClick={() => toggleExpand(p.id)}
                >
                  <td><span className="badge badge-type">{p.proposalType}</span></td>
                  <td>{p.summary}</td>
                  <td>{fmtDate(p.createdAt)}</td>
                  <td><span className={`badge badge-status status-${p.status}`}>{p.status}</span></td>
                  <td onClick={e => e.stopPropagation()}>
                    {p.status === 'pending' && (
                      <div className="action-btns">
                        <button
                          className="btn-approve"
                          disabled={resolving === p.id}
                          onClick={() => resolve(p, 'approved')}
                        >
                          ✓ Approve
                        </button>
                        <button
                          className="btn-reject"
                          disabled={resolving === p.id}
                          onClick={() => resolve(p, 'rejected')}
                        >
                          ✗ Reject
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
                {expanded === p.id && (
                  <tr key={`${p.id}-exp`} className="expanded-row">
                    <td colSpan={5}>
                      <div className="expanded-content">
                        {p.suggestedText && (
                          <div className="expand-section">
                            <strong>Suggested Text</strong>
                            <pre className="pre-wrap">{p.suggestedText}</pre>
                          </div>
                        )}
                        {p.memoryIds?.length > 0 && (
                          <div className="expand-section">
                            <strong>Memory IDs</strong>
                            <ul className="id-list">
                              {p.memoryIds.map(id => <li key={id} className="mono-small">{id}</li>)}
                            </ul>
                          </div>
                        )}
                        {p.details && Object.keys(p.details).length > 0 && (
                          <div className="expand-section">
                            <strong>Details</strong>
                            <pre className="pre-wrap">{JSON.stringify(p.details, null, 2)}</pre>
                          </div>
                        )}
                        <p className="meta">ID: {p.id} · Reviewed By: {p.reviewedBy || 'none'}</p>
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
