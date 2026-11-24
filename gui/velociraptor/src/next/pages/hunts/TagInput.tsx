import { useId, useState } from 'react';
import { X } from 'lucide-react';
import { cn } from '../../ui/primitives';

// Free-form chip input with optional suggestions (via <datalist>).
export function TagInput({ value, onChange, suggestions = [], placeholder, tone = 'neutral' }: {
  value: string[]; onChange: (v: string[]) => void; suggestions?: string[]; placeholder?: string;
  tone?: 'neutral' | 'accent' | 'bad';
}) {
  const [draft, setDraft] = useState('');
  const listId = useId();
  const add = (raw: string) => {
    const t = raw.trim().replace(/,$/, '');
    if (t && !value.includes(t)) onChange([...value, t]);
    setDraft('');
  };
  const chip = tone === 'accent' ? 'tw:bg-accent-soft tw:text-accent'
    : tone === 'bad' ? 'tw:bg-bad-soft tw:text-bad' : 'tw:bg-hover tw:text-fg';
  return (
    <div className={cn(
      'tw:flex tw:flex-wrap tw:items-center tw:gap-1 tw:min-h-8 tw:w-full tw:rounded-md tw:border tw:border-line-strong tw:bg-panel tw:px-1.5 tw:py-1',
      'tw:focus-within:border-accent tw:focus-within:ring-2 tw:focus-within:ring-accent/20')}>
      {value.map(t => (
        <span key={t} className={cn('tw:inline-flex tw:items-center tw:gap-1 tw:h-5 tw:rounded tw:px-1.5 tw:text-xs tw:font-medium', chip)}>
          {t}
          <button type="button" aria-label={`Remove ${t}`} onClick={() => onChange(value.filter(x => x !== t))}
            className="tw:opacity-60 tw:hover:opacity-100"><X className="tw:size-3" /></button>
        </span>
      ))}
      <input value={draft} list={suggestions.length ? listId : undefined}
        onChange={e => {
          const v = e.target.value;
          if (v.endsWith(',')) add(v);
          else setDraft(v);
        }}
        onKeyDown={e => {
          if (e.key === 'Enter') {
            e.preventDefault();
            add(draft);
          } else if (e.key === 'Backspace' && !draft && value.length) {
            onChange(value.slice(0, -1));
          }
        }}
        onBlur={() => draft && add(draft)}
        placeholder={value.length ? '' : placeholder}
        className="tw:flex-1 tw:min-w-[100px] tw:h-6 tw:bg-transparent tw:outline-none tw:text-[13px] tw:px-1 tw:placeholder:text-faint" />
      {suggestions.length > 0 && (
        <datalist id={listId}>
          {suggestions.filter(s => !value.includes(s)).map(s => <option key={s} value={s} />)}
        </datalist>
      )}
    </div>
  );
}
