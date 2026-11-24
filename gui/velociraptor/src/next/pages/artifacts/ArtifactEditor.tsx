// Full-screen YAML editor for custom artifacts with server-side checks.
import { lazy, Suspense, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { Dialog as RDialog } from 'radix-ui';
import { CircleAlert, Code2, Save, TriangleAlert, X } from 'lucide-react';
import { useArtifactFile, useSetArtifactFile } from '../../lib/queries';
import { Button, Kbd, Spinner } from '../../ui/primitives';

const CodeEditor = lazy(() => import('../../ui/CodeEditor.jsx'));

const TEMPLATE = `name: Custom.Artifact.Name
description: |
   What this artifact collects and why.

# Can be CLIENT, CLIENT_EVENT, SERVER, SERVER_EVENT or NOTEBOOK
type: CLIENT

parameters:
   - name: FileGlob
     default: C:/Users/**/*.exe
     description: Files to look at

sources:
  - precondition:
      SELECT OS From info() where OS = 'windows' OR OS = 'linux' OR OS = 'darwin'

    query: |
      SELECT OSPath, Size, Mtime
      FROM glob(globs=FileGlob)
`;

export default function ArtifactEditor({ name, template, onClose }: {
  name?: string; template?: 'copy' | string; onClose: (saved?: string) => void;
}) {
  const file = useArtifactFile(name);
  const set = useSetArtifactFile();
  const [text, setText] = useState<string | null>(name ? null : TEMPLATE);
  const [problems, setProblems] = useState<{ errors: string[]; warnings: string[] }>({ errors: [], warnings: [] });

  useEffect(() => {
    if (text === null && file.data !== undefined) {
      let t = file.data;
      // Customizing a built-in artifact: save as a new name.
      if (template === 'copy' && name) t = t.replace(/^name:\s*.+$/m, `name: Custom.${name}`);
      setText(t);
    }
  }, [file.data, text, template, name]);

  const save = () => {
    if (!text) return;
    set.mutate({ artifact: text, op: 'CHECK_AND_SET' }, {
      onSuccess: r => {
        const errors = r?.errors ?? [];
        const warnings = r?.warnings ?? [];
        if (errors.length || warnings.length) {
          setProblems({ errors, warnings });
          if (!errors.length) {
            toast.warning('Saved with warnings');
          }
          return;
        }
        const saved = text.match(/^name:\s*(.+)$/m)?.[1]?.trim();
        toast.success(`Saved ${saved ?? 'artifact'}`);
        onClose(saved);
      },
      onError: e => setProblems({ errors: [(e as Error).message], warnings: [] }),
    });
  };

  const title = !name ? 'New artifact' : template === 'copy' ? `Customize ${name}` : `Edit ${name}`;

  return (
    <RDialog.Root open onOpenChange={o => !o && onClose()}>
      <RDialog.Portal>
        <RDialog.Overlay className="rx tw:fixed tw:inset-0 tw:z-[1500] tw:bg-black/40 tw:animate-fade-in" />
        <RDialog.Content aria-describedby={undefined}
          onKeyDown={e => { if ((e.metaKey || e.ctrlKey) && e.key === 's') { e.preventDefault(); save(); } }}
          className="rx tw:fixed tw:left-1/2 tw:top-1/2 tw:z-[1501] tw:-translate-x-1/2 tw:-translate-y-1/2 tw:w-[1100px] tw:max-w-[calc(100vw-32px)] tw:h-[calc(100vh-48px)] tw:flex tw:flex-col tw:bg-panel tw:rounded-xl tw:border tw:border-line rx-shadow-lg tw:outline-none">
          <div className="tw:flex tw:items-center tw:gap-3 tw:px-5 tw:h-14 tw:border-b tw:border-line tw:shrink-0">
            <span className="tw:flex tw:size-8 tw:items-center tw:justify-center tw:rounded-lg tw:bg-accent-soft tw:text-accent"><Code2 className="tw:size-4" /></span>
            <RDialog.Title className="tw:text-[15px] tw:font-semibold tw:flex-1 tw:truncate">{title}</RDialog.Title>
            <RDialog.Close className="tw:p-1.5 tw:rounded tw:text-faint tw:hover:bg-hover tw:hover:text-fg" aria-label="Close"><X className="tw:size-4" /></RDialog.Close>
          </div>
          {(problems.errors.length > 0 || problems.warnings.length > 0) && (
            <div className="tw:px-5 tw:py-2 tw:space-y-1 tw:border-b tw:border-line tw:max-h-40 tw:overflow-auto">
              {problems.errors.map((e, i) => <div key={`e${i}`} className="tw:flex tw:gap-2 tw:text-xs tw:text-bad"><CircleAlert className="tw:size-3.5 tw:shrink-0 tw:mt-0.5" />{e}</div>)}
              {problems.warnings.map((e, i) => <div key={`w${i}`} className="tw:flex tw:gap-2 tw:text-xs tw:text-warn"><TriangleAlert className="tw:size-3.5 tw:shrink-0 tw:mt-0.5" />{e}</div>)}
            </div>
          )}
          <div className="tw:flex-1 tw:min-h-0 tw:p-3">
            {text === null
              ? <div className="tw:p-6"><Spinner /></div>
              : <Suspense fallback={<Spinner />}>
                  <CodeEditor value={text} onChange={(v: string) => setText(v)} mode="yaml" focus className="rx-code-fill" />
                </Suspense>}
          </div>
          <div className="tw:flex tw:items-center tw:gap-2 tw:px-5 tw:h-14 tw:border-t tw:border-line tw:shrink-0">
            <span className="tw:text-2xs tw:text-faint">Custom artifact names should start with <span className="tw:font-mono">Custom.</span></span>
            <div className="tw:ml-auto tw:flex tw:items-center tw:gap-2">
              <span className="tw:text-2xs tw:text-faint tw:flex tw:items-center tw:gap-1"><Kbd>Ctrl</Kbd><Kbd>S</Kbd></span>
              <Button variant="ghost" onClick={() => onClose()}>Cancel</Button>
              <Button variant="primary" icon={<Save className="tw:size-3.5" />} loading={set.isPending} disabled={!text} onClick={save}>Check and save</Button>
            </div>
          </div>
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}
