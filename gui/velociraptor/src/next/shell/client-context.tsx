import React, { createContext, useCallback, useContext, useMemo, useState } from 'react';
import { setItem, schema } from '../../components/core/storage.jsx';
import type { ApiClient } from '../lib/types';

// The client currently "in focus". Legacy screens read and write it
// through props (client / setClient); the new shell owns the state.
export interface ClientCtx {
  client: Partial<ApiClient> & { client_id?: string | null };
  setClient: (c: ClientCtx['client']) => void;
}

const Ctx = createContext<ClientCtx>({ client: {}, setClient: () => undefined });

export function ClientProvider({ children }: { children: React.ReactNode }) {
  const [client, setClientState] = useState<ClientCtx['client']>({});
  const setClient = useCallback((c: ClientCtx['client']) => {
    const id = c?.client_id;
    if (id) setItem(schema.CurrentSelectedClientKey, id);
    setClientState(c || {});
  }, []);
  const value = useMemo(() => ({ client, setClient }), [client, setClient]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSelectedClient() {
  return useContext(Ctx);
}
