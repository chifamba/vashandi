import { useState } from 'react';

interface Props {
  onLogin: () => void;
}

export default function Login({ onLogin }: Props) {
  const [baseUrl, setBaseUrl] = useState(
    sessionStorage.getItem('ob_base_url') ?? 'http://localhost:3101'
  );
  const [token, setToken] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const res = await fetch(`${baseUrl}/api/v1/health`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) throw new Error(`Server responded ${res.status}`);
      sessionStorage.setItem('ob_base_url', baseUrl);
      sessionStorage.setItem('ob_token', token);
      onLogin();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Connection failed');
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="login-wrap">
      <div className="login-card">
        <h1 className="login-title">OpenBrain Admin</h1>
        <p className="login-sub">Enter your API credentials to continue</p>
        <form onSubmit={handleSubmit} className="login-form">
          <label>
            Base URL
            <input
              type="text"
              value={baseUrl}
              onChange={e => setBaseUrl(e.target.value)}
              required
              placeholder="http://localhost:3101"
            />
          </label>
          <label>
            API Token
            <input
              type="password"
              value={token}
              onChange={e => setToken(e.target.value)}
              required
              placeholder="Bearer token"
            />
          </label>
          {error && <p className="error-msg">{error}</p>}
          <button type="submit" disabled={loading} className="btn-primary">
            {loading ? 'Connecting…' : 'Connect'}
          </button>
        </form>
      </div>
    </div>
  );
}
