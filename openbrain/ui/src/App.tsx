import { useState } from 'react';
import Login from './Login';
import Dashboard from './Dashboard';
import Memories from './Memories';
import Proposals from './Proposals';
import AuditLog from './AuditLog';
import Agents from './Agents';

type Tab = 'dashboard' | 'memories' | 'proposals' | 'audit' | 'agents';

const TABS: { id: Tab; label: string }[] = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'memories', label: 'Memories' },
  { id: 'proposals', label: 'Proposals' },
  { id: 'audit', label: 'Audit Log' },
  { id: 'agents', label: 'Agents' },
];

function isLoggedIn() {
  return !!sessionStorage.getItem('ob_token');
}

export default function App() {
  const [loggedIn, setLoggedIn] = useState(isLoggedIn);
  const [tab, setTab] = useState<Tab>('dashboard');
  const [namespaceId, setNamespaceId] = useState(
    sessionStorage.getItem('ob_namespace') ?? ''
  );

  function handleNamespaceChange(ns: string) {
    setNamespaceId(ns);
    sessionStorage.setItem('ob_namespace', ns);
  }

  function logout() {
    sessionStorage.clear();
    setLoggedIn(false);
  }

  if (!loggedIn) {
    return <Login onLogin={() => setLoggedIn(true)} />;
  }

  return (
    <div className="app">
      <header className="app-header">
        <span className="app-brand">⬡ OpenBrain Admin</span>
        <nav className="app-nav">
          {TABS.map(t => (
            <button
              key={t.id}
              className={`nav-tab ${tab === t.id ? 'active' : ''}`}
              onClick={() => setTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </nav>
        <button className="btn-logout" onClick={logout}>Logout</button>
      </header>

      <main className="app-main">
        {tab === 'dashboard' && (
          <Dashboard namespaceId={namespaceId} onNamespaceChange={handleNamespaceChange} />
        )}
        {tab === 'memories' && <Memories namespaceId={namespaceId} />}
        {tab === 'proposals' && <Proposals namespaceId={namespaceId} />}
        {tab === 'audit' && <AuditLog namespaceId={namespaceId} />}
        {tab === 'agents' && <Agents namespaceId={namespaceId} />}
      </main>
    </div>
  );
}
