// "Export hunt" dialog: choose what goes in the zip, then the archive is
// built in the background with live progress in the hunt's Exports tab.
import { useState } from 'react';
import { toast } from 'sonner';
import { Check, Download, FileJson, FileSpreadsheet, FolderArchive, Lock, Sheet } from 'lucide-react';
import type { Hunt } from '../../lib/types';
import { useExportHunt, useTraits } from '../../lib/queries';
import { fmtBytes, fmtNum, toNum } from '../../lib/format';
import { Button, cn, Dialog, Switch, Tooltip } from '../../ui/primitives';

type Scope = 'full' | 'combined';

export default function ExportHuntDialog({ hunt, open, onOpenChange, onStarted }: {
  hunt: Hunt; open: boolean; onOpenChange: (v: boolean) => void; onStarted?: () => void;
}) {
  const { traits } = useTraits();
  const hasPassword = !!traits?.default_password;
  const [scope, setScope] = useState<Scope>('full');
  const [json, setJson] = useState(true);
  const [csv, setCsv] = useState(true);
  const [lock, setLock] = useState(hasPassword);
  const exp = useExportHunt();
  const st = hunt.stats ?? {};
  const clients = toNum(st.total_clients_with_results) || toNum(st.total_finished_clients);
  const rows = toNum(st.total_collected_rows);
  const bytes = toNum(st.total_uploaded_bytes);

  const submit = () => {
    exp.mutate({
      hunt_id: hunt.hunt_id,
      only_combined_hunt: scope === 'combined',
      json_format: json,
      csv_format: csv,
      password: lock && hasPassword ? traits?.default_password : '',
    }, {
      onSuccess: () => {
        toast.success('Export started', { description: 'The zip downloads automatically when it is ready. You can keep working.' });
        onOpenChange(false);
        onStarted?.();
      },
      onError: e => toast.error((e as Error).message),
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange} width={620}
      title="Export hunt"
      description="Package the evidence collected by this hunt into a single zip file."
      footer={<>
        <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button variant="primary" icon={<Download className="tw:size-3.5" />} loading={exp.isPending}
          disabled={!json && !csv} onClick={submit}>Prepare export</Button>
      </>}>
      <div className="tw:space-y-5">
        <div className="tw:grid tw:grid-cols-1 tw:sm:grid-cols-2 tw:gap-2.5">
          <ScopeCard active={scope === 'full'} onClick={() => setScope('full')}
            icon={<FolderArchive />} title="Full evidence"
            text="Every client’s collection with all uploaded files, logs and results, plus combined results across clients." />
          <ScopeCard active={scope === 'combined'} onClick={() => setScope('combined')}
            icon={<Sheet />} title="Combined results only"
            text="One table per artifact with rows from all clients. Small and quick, without uploaded files." />
        </div>

        <div className="tw:rounded-lg tw:border tw:border-line tw:bg-canvas/60 tw:px-3 tw:py-2.5 tw:text-xs tw:text-muted tw:flex tw:flex-wrap tw:gap-x-5 tw:gap-y-1">
          <span><b className="tw:text-fg rx-num">{fmtNum(clients)}</b> client{clients === 1 ? '' : 's'}</span>
          <span><b className="tw:text-fg rx-num">{fmtNum(rows)}</b> rows</span>
          {scope === 'full'
            ? <span><b className="tw:text-fg rx-num">{fmtBytes(bytes)}</b> of uploaded files</span>
            : <span className="tw:text-faint">uploaded files not included</span>}
        </div>

        <div className="tw:space-y-2">
          <div className="tw:text-xs tw:font-medium">Result formats</div>
          <div className="tw:flex tw:gap-2">
            <FormatChip on={json} onClick={() => setJson(!json)} icon={<FileJson />} label="JSON" />
            <FormatChip on={csv} onClick={() => setCsv(!csv)} icon={<FileSpreadsheet />} label="CSV" />
          </div>
          {!json && !csv && <div className="tw:text-2xs tw:text-bad">Pick at least one format.</div>}
        </div>

        <div className="tw:flex tw:items-start tw:gap-3 tw:rounded-lg tw:border tw:border-line tw:px-3 tw:py-2.5">
          <Lock className="tw:size-4 tw:mt-0.5 tw:text-faint" />
          <div className="tw:flex-1">
            <div className="tw:text-[13px] tw:font-medium">Password-protect the zip</div>
            <div className="tw:text-2xs tw:text-muted">
              {hasPassword
                ? 'Uses the default download password from your user settings. Recommended for malware and sensitive files.'
                : 'Set a default download password in your user settings to enable this.'}
            </div>
          </div>
          <Tooltip content={hasPassword ? '' : 'No default password set'}>
            <span><Switch checked={lock && hasPassword} onChange={setLock} disabled={!hasPassword} /></span>
          </Tooltip>
        </div>
      </div>
    </Dialog>
  );
}

function ScopeCard({ active, onClick, icon, title, text }: {
  active: boolean; onClick: () => void; icon: React.ReactNode; title: string; text: string;
}) {
  return (
    <button type="button" onClick={onClick}
      className={cn('tw:relative tw:text-left tw:rounded-lg tw:border tw:p-3 tw:transition-colors',
        active ? 'tw:border-accent tw:bg-accent-soft' : 'tw:border-line tw:hover:border-line-strong tw:hover:bg-hover')}>
      <div className="tw:flex tw:items-center tw:gap-2">
        <span className={cn('tw:flex tw:[&>svg]:size-4', active ? 'tw:text-accent' : 'tw:text-faint')}>{icon}</span>
        <span className="tw:text-[13px] tw:font-semibold">{title}</span>
        {active && <Check className="tw:ml-auto tw:size-4 tw:text-accent" />}
      </div>
      <p className="tw:mt-1.5 tw:text-2xs tw:text-muted tw:leading-relaxed">{text}</p>
    </button>
  );
}

function FormatChip({ on, onClick, icon, label }: { on: boolean; onClick: () => void; icon: React.ReactNode; label: string }) {
  return (
    <button type="button" onClick={onClick} aria-pressed={on}
      className={cn('tw:flex tw:items-center tw:gap-1.5 tw:h-8 tw:px-3 tw:rounded-md tw:border tw:text-xs tw:font-medium tw:[&>svg]:size-3.5',
        on ? 'tw:border-accent tw:bg-accent-soft tw:text-accent' : 'tw:border-line-strong tw:text-muted tw:hover:bg-hover')}>
      {on ? <Check /> : icon}{label}
    </button>
  );
}
