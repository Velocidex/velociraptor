import { useState } from 'react';
import { toast } from 'sonner';
import type { Hunt } from '../../lib/types';
import { useHuntTags, useModifyHunt } from '../../lib/queries';
import { toDate } from '../../lib/format';
import { Button, Dialog, Field, Input } from '../../ui/primitives';
import { TagInput } from './TagInput';
import { toLocalInput, fromLocalInput } from './time';

export default function EditHuntDialog({ hunt, open, onOpenChange }: {
  hunt: Hunt; open: boolean; onOpenChange: (v: boolean) => void;
}) {
  const [description, setDescription] = useState(hunt.hunt_description ?? '');
  const [tags, setTags] = useState<string[]>(hunt.tags ?? []);
  const [expires, setExpires] = useState(toLocalInput(toDate(hunt.expires)));
  const { data: allTags = [] } = useHuntTags();
  const modify = useModifyHunt();

  const save = () => {
    const exp = fromLocalInput(expires);
    let newTags = tags;
    // An explicit "-" tells the server to clear all tags.
    if (tags.length === 0 && (hunt.tags?.length ?? 0) > 0) newTags = ['-'];
    modify.mutate({
      hunt_id: hunt.hunt_id,
      description: description || ' ',
      tags: newTags,
      expires: exp ? exp.getTime() * 1000 : undefined,
    }, {
      onSuccess: () => {
        toast.success('Hunt updated');
        onOpenChange(false);
      },
      onError: e => toast.error((e as Error).message),
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange} title="Edit hunt" description={hunt.hunt_id} width={520}
      footer={<>
        <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button variant="primary" loading={modify.isPending} onClick={save}>Save changes</Button>
      </>}>
      <div className="tw:space-y-4">
        <Field label="Description">
          <Input value={description} onChange={e => setDescription(e.target.value)} autoFocus />
        </Field>
        <Field label="Tags" hint="Press Enter to add a tag.">
          <TagInput value={tags} onChange={setTags} suggestions={allTags} />
        </Field>
        <Field label="Expires">
          <Input type="datetime-local" value={expires} onChange={e => setExpires(e.target.value)} />
        </Field>
      </div>
    </Dialog>
  );
}
