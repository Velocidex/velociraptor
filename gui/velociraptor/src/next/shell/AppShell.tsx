import { lazy, Suspense, useEffect, useState } from 'react';
import { Redirect, Route, Switch } from 'react-router-dom';
import Sidebar from './Sidebar';
import TopBar from './TopBar';
import CommandPalette from './CommandPalette';
import { useSelectedClient } from './client-context';
import { useThemeSync } from './theme';
import { Spinner } from '../ui/primitives';
import { ErrorBoundary } from './ErrorBoundary';

const Dashboard = lazy(() => import('../pages/dashboard/Dashboard'));
const HuntsPage = lazy(() => import('../pages/hunts/HuntsPage'));
const ClientsPage = lazy(() => import('../pages/clients/ClientsPage'));
const LegacyRoutes = lazy(() => import('./LegacyRoutes.jsx'));

function PageFallback() {
  return (
    <div className="tw:flex tw:flex-1 tw:items-center tw:justify-center">
      <Spinner className="tw:size-5" />
    </div>
  );
}

function readCollapsed(): boolean {
  try {
    return localStorage.getItem('rx-sidebar-collapsed') === '1';
  } catch {
    return false;
  }
}

export default function AppShell() {
  const dark = useThemeSync();
  const { client, setClient } = useSelectedClient();
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [paletteOpen, setPaletteOpen] = useState(false);

  useEffect(() => {
    try {
      localStorage.setItem('rx-sidebar-collapsed', collapsed ? '1' : '0');
    } catch {
      // ignore
    }
  }, [collapsed]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setPaletteOpen(o => !o);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  return (
    <div className="rx tw:fixed tw:inset-0 tw:flex tw:bg-canvas tw:text-fg">
      <Sidebar collapsed={collapsed} onToggle={() => setCollapsed(c => !c)} />
      <div className="tw:flex tw:flex-col tw:flex-1 tw:min-w-0">
        <TopBar onOpenPalette={() => setPaletteOpen(true)} dark={dark} />
        <main className="tw:flex tw:flex-col tw:flex-1 tw:min-h-0 tw:relative">
          <ErrorBoundary>
            <Suspense fallback={<PageFallback />}>
              <Switch>
                <Route exact path={['/', '/welcome', '/dashboard']}>
                  <Dashboard />
                </Route>
                <Route path="/hunts/:hunt_id?/:tab?">
                  <HuntsPage />
                </Route>
                <Route path="/search/:query?">
                  <ClientsPage />
                </Route>
                <Route path="/classic" exact>
                  <Redirect to="/classic/hunts" />
                </Route>
                <Route>
                  <LegacyRoutes client={client} setClient={setClient} />
                </Route>
              </Switch>
            </Suspense>
          </ErrorBoundary>
        </main>
      </div>
      <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} dark={dark} />
    </div>
  );
}
