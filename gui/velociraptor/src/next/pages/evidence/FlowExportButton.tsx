// One-click "download this collection": builds the zip on the server,
// waits for it and starts the download.
import { Download } from 'lucide-react';
import { useTraits } from '../../lib/queries';
import { downloadFlowZip, useFlowZipBusy } from '../../lib/flowZip';
import { Button, IconButton } from '../../ui/primitives';

export function useFlowDownloader(clientId: string, flowId: string, label?: string) {
  const { traits } = useTraits();
  const busy = useFlowZipBusy(clientId, flowId);
  const start = () => { void downloadFlowZip(clientId, flowId, { password: traits?.default_password, label }); };
  return { start, busy };
}

export function FlowDownloadButton({ clientId, flowId, compact, label = 'Download zip', name, variant = 'secondary' }: {
  clientId: string; flowId: string; compact?: boolean; label?: string; name?: string;
  variant?: 'primary' | 'secondary';
}) {
  const { start, busy } = useFlowDownloader(clientId, flowId, name);
  if (compact) {
    return (
      <IconButton size="xs" label={busy ? 'Building zip…' : 'Download this collection as a zip'}
        icon={<Download className="tw:size-3.5" />} loading={busy}
        onClick={e => { e.stopPropagation(); start(); }} />
    );
  }
  return (
    <Button size="sm" variant={variant} icon={<Download className="tw:size-3.5" />} loading={busy} onClick={start}>
      {busy ? 'Building zip…' : label}
    </Button>
  );
}
