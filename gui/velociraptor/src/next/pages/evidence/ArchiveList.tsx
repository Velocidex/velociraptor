// Prepared export archives (zip files) for a hunt or a collection, with
// live progress while the server builds them.
import { useEffect, useRef } from 'react';
import { toast } from 'sonner';
import { CircleAlert, Download, FileArchive, Lock } from 'lucide-react';
import type { AvailableDownloadFile } from '../../lib/types';
import { archiveHref, archiveState, archiveTime, sortArchives, triggerDownload } from '../../lib/downloads';
import { fmtBytes, fmtDuration, fmtNum, fmtTime, timeAgo, toNum } from '../../lib/format';
import { Badge, Button, CopyButton, EmptyState, Mono, Spinner, Tooltip } from '../../ui/primitives';

// Shows a toast with a download button whenever an archive that was
// being prepared while the page was open becomes ready.
export function useArchiveReadyToast(files: AvailableDownloadFile[], autoDownload?: React.MutableRefObject<Set<string>>) {
  const prev = useRef<Map<string, string>>(new Map());
  const mountedAt = useRef(Math.floor(Date.now() / 1000));
  useEffect(() => {
    const now = new Map(files.map(f => [`${f.path || f.name}@${archiveTime(f)}`, archiveState(f)]));
    const before = prev.current;
    prev.current = now;
    for (const f of files) {
      const key = `${f.path || f.name}@${archiveTime(f)}`;
      const was = before.get(key);
      const is = now.get(key);
      // Only announce archives that were started while this page was open.
      const startedHere = was === 'preparing' || (was === undefined && archiveTime(f) >= mountedAt.current - 2);
      if (is === 'ready' && was !== 'ready' && startedHere) {
        if (autoDownload?.current.has(key) || (autoDownload && autoDownload.current.has('*'))) {
          autoDownload.current.delete(key);
          autoDownload.current.delete('*');
          triggerDownload(archiveHref(f));
          toast.success(`Downloading ${f.name}.zip`, { description: fmtBytes(f.size ?? f.stats?.total_compressed_bytes) });
        } else {
          toast.success('Export ready', {
            description: `${f.name} · ${fmtBytes(f.size ?? f.stats?.total_compressed_bytes)}`,
            action: { label: 'Download', onClick: () => triggerDownload(archiveHref(f)) },
            duration: 15_000,
          });
        }
      } else if (is === 'failed' && startedHere) {
        toast.error(`Export failed: ${f.stats?.error}`);
      }
    }
  }, [files, autoDownload]);
}

export function ArchiveList({ files, empty, locked }: {
  files: AvailableDownloadFile[]; empty?: React.ReactNode; locked?: boolean;
}) {
  if (!files.length) {
    return <>{empty ?? <EmptyState icon={<FileArchive />} title="No exports yet" />}</>;
  }
  return (
    <div className="tw:divide-y tw:divide-line">
      {sortArchives(files).map(f => <ArchiveRow key={`${f.path || f.name}@${archiveTime(f)}`} f={f} locked={locked} />)}
    </div>
  );
}

function ArchiveRow({ f, locked }: { f: AvailableDownloadFile; locked?: boolean }) {
  const st = archiveState(f);
  const s = f.stats ?? {};
  const ts = archiveTime(f);
  const size = toNum(f.size ?? s.total_compressed_bytes);
  return (
    <div className="tw:flex tw:items-start tw:gap-3 tw:px-4 tw:py-3">
      <span className={`tw:mt-0.5 tw:flex tw:size-8 tw:shrink-0 tw:items-center tw:justify-center tw:rounded-lg ${
        st === 'ready' ? 'tw:bg-ok-soft tw:text-ok' : st === 'failed' ? 'tw:bg-bad-soft tw:text-bad' : 'tw:bg-accent-soft tw:text-accent'}`}>
        {st === 'preparing' ? <Spinner className="tw:size-4 tw:text-accent" /> : st === 'failed' ? <CircleAlert className="tw:size-4" /> : <FileArchive className="tw:size-4" />}
      </span>
      <div className="tw:min-w-0 tw:flex-1">
        <div className="tw:flex tw:items-center tw:gap-2 tw:flex-wrap">
          <span className="tw:text-[13px] tw:font-medium tw:break-all">{f.name}.zip</span>
          {st === 'preparing' && <Badge tone="accent">Preparing</Badge>}
          {st === 'failed' && <Badge tone="bad">Failed</Badge>}
          {locked && <Tooltip content="Password protected"><Lock className="tw:size-3 tw:text-faint" /></Tooltip>}
        </div>
        <div className="tw:mt-1 tw:flex tw:items-center tw:gap-x-3 tw:gap-y-0.5 tw:flex-wrap tw:text-2xs tw:text-muted">
          {st === 'preparing' ? (
            <span>{fmtBytes(s.total_uncompressed_bytes)} written · {fmtNum(s.total_container_files)} files so far</span>
          ) : st === 'failed' ? (
            <span className="tw:text-bad">{s.error}</span>
          ) : (
            <>
              <span><b className="tw:font-semibold tw:text-fg rx-num">{fmtBytes(size)}</b> zip</span>
              <span>{fmtNum(s.total_container_files)} files</span>
              <span>{fmtBytes(s.total_uncompressed_bytes)} uncompressed</span>
              {toNum(s.total_duration) > 0 && <span>built in {fmtDuration(toNum(s.total_duration))}</span>}
            </>
          )}
          {ts > 0 && <Tooltip content={fmtTime(ts)}><span>{timeAgo(ts)}</span></Tooltip>}
        </div>
        {s.hash && (
          <div className="tw:mt-1 tw:flex tw:items-center tw:gap-1 tw:text-2xs tw:text-faint">
            SHA-256 <Mono className="tw:text-[11px] tw:truncate tw:max-w-[420px]">{s.hash}</Mono><CopyButton text={s.hash} />
          </div>
        )}
        {st === 'preparing' && (
          <div className="tw:mt-2 tw:h-1 tw:w-full tw:max-w-[420px] tw:overflow-hidden tw:rounded-full tw:bg-hover">
            <div className="tw:h-full tw:w-1/3 tw:rounded-full tw:bg-accent rx-indeterminate" />
          </div>
        )}
      </div>
      {st === 'ready' && (
        <Button size="sm" variant="secondary" icon={<Download className="tw:size-3.5" />}
          onClick={() => triggerDownload(archiveHref(f))}>Download</Button>
      )}
    </div>
  );
}
