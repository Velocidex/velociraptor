import { lazy, Suspense } from 'react';
import { HashRouter, Route, Switch } from 'react-router-dom';
import { QueryClientProvider } from '@tanstack/react-query';
import { Toaster } from 'sonner';
import { queryClient } from './lib/queries';
import { ClientProvider, useSelectedClient } from './shell/client-context';
import { TooltipProvider } from './ui/primitives';
import AppShell from './shell/AppShell';
import { isDarkTheme } from './shell/theme';
// Order matters: the new UI's styles must load after the legacy CSS.
import './shell/legacy-styles.js';
import './styles.css';

import { UserSettings } from '../components/core/user.jsx';
import Snackbar from '../components/core/snackbar.jsx';
import KeyboardHelp from '../components/core/keyboard-help.jsx';
import SidebarKeyNavigator from '../components/sidebar/hotkeys.jsx';

// Pages shown without the shell (login, logoff, full screen views).
const LoginPage = lazy(() => import('../components/welcome/login.jsx'));
const LogoffPage = lazy(() => import('../components/welcome/logoff.jsx'));
const LegacyFullscreen = lazy(() => import('./shell/LegacyFullscreen.jsx'));

function Shell() {
  const { client } = useSelectedClient();
  return (
    <UserSettings>
      <SidebarKeyNavigator client={client} />
      <Switch>
        <Route path="/fullscreen">
          <Suspense fallback={null}>
            <LegacyFullscreen />
          </Suspense>
        </Route>
        <Route>
          <AppShell />
        </Route>
      </Switch>
      <KeyboardHelp />
      <Snackbar />
    </UserSettings>
  );
}

export default function Root() {
  const errorState = window.ErrorState;
  if (errorState && (errorState.Type === 'Login' || errorState.Type === 'Logoff')) {
    return (
      <HashRouter>
        <Suspense fallback={null}>
          {errorState.Type === 'Login' ? <LoginPage /> : <LogoffPage />}
          <Snackbar />
        </Suspense>
      </HashRouter>
    );
  }

  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider delayDuration={300}>
        <HashRouter>
          <ClientProvider>
            <Shell />
          </ClientProvider>
        </HashRouter>
        <Toaster position="bottom-right" theme={isDarkTheme(undefined) ? 'dark' : 'light'} richColors closeButton
          toastOptions={{ className: 'rx' }} />
      </TooltipProvider>
    </QueryClientProvider>
  );
}
