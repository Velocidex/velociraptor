import React, { forwardRef } from 'react';
import { Dialog as RDialog, DropdownMenu as RMenu, Tabs as RTabs, Tooltip as RTooltip } from 'radix-ui';
import { Loader2, X, Search } from 'lucide-react';
import { extendTailwindMerge } from 'tailwind-merge';

// Utilities are !important, so a later class cannot simply override an
// earlier one by cascade order. tailwind-merge drops the conflicting
// earlier class instead (e.g. "tw:h-8 tw:h-7" -> "tw:h-7").
const merge = extendTailwindMerge({
  prefix: 'tw',
  extend: {
    theme: {
      color: ['canvas', 'panel', 'raised', 'hover', 'active', 'line', 'line-strong', 'fg', 'muted', 'faint',
        'accent', 'accent-fg', 'accent-soft', 'ok', 'ok-soft', 'warn', 'warn-soft', 'bad', 'bad-soft',
        'info', 'info-soft', 'sidebar'],
      text: ['2xs'],
    },
  },
});

export function cn(...parts: (string | false | null | undefined)[]): string {
  return merge(parts.filter(Boolean).join(' '));
}

// ---------------------------------------------------------------- Button

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'subtle';
type ButtonSize = 'xs' | 'sm' | 'md';

const buttonVariants: Record<ButtonVariant, string> = {
  primary: 'tw:bg-accent tw:text-accent-fg tw:hover:brightness-110 tw:shadow-sm',
  secondary: 'tw:bg-panel tw:text-fg tw:border tw:border-line-strong tw:hover:bg-hover tw:shadow-sm',
  ghost: 'tw:text-muted tw:hover:bg-hover tw:hover:text-fg',
  subtle: 'tw:bg-hover tw:text-fg tw:hover:bg-active',
  danger: 'tw:bg-bad tw:text-white tw:hover:brightness-110 tw:shadow-sm',
};

const buttonSizes: Record<ButtonSize, string> = {
  xs: 'tw:h-6 tw:px-2 tw:text-xs tw:gap-1',
  sm: 'tw:h-7 tw:px-2.5 tw:text-xs tw:gap-1.5',
  md: 'tw:h-8 tw:px-3 tw:text-[13px] tw:gap-2',
};

// Icons passed without an explicit size get one that fits the button.
const iconSizes: Record<ButtonSize, string> = {
  xs: 'tw:[&>svg:not([class*=size-])]:size-3',
  sm: 'tw:[&>svg:not([class*=size-])]:size-3.5',
  md: 'tw:[&>svg:not([class*=size-])]:size-4',
};

// Square, icon-only buttons. Kept separate because utilities are
// !important, so conflicting padding classes cannot be overridden.
const squareSizes: Record<ButtonSize, string> = {
  xs: 'tw:size-6',
  sm: 'tw:size-7',
  md: 'tw:size-8',
};

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  icon?: React.ReactNode;
  square?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', loading, icon, square, className, children, disabled, ...rest }, ref) {
  return (
    <button
      ref={ref}
      disabled={disabled || loading}
      className={cn(
        'tw:inline-flex tw:items-center tw:justify-center tw:rounded-md tw:font-medium tw:whitespace-nowrap',
        'tw:transition-colors tw:select-none tw:disabled:opacity-50 tw:disabled:pointer-events-none',
        buttonVariants[variant], square ? squareSizes[size] : buttonSizes[size], iconSizes[size], className)}
      {...rest}
    >
      {loading ? <Loader2 className="tw:size-3.5 tw:animate-spin" /> : icon}
      {children}
    </button>
  );
});

export const IconButton = forwardRef<HTMLButtonElement, ButtonProps & { label: string }>(
  function IconButton({ label, size = 'md', variant = 'ghost', ...rest }, ref) {
    return (
      <Tooltip content={label}>
        <Button ref={ref} aria-label={label} variant={variant} size={size} square {...rest} />
      </Tooltip>
    );
  });

// ----------------------------------------------------------------- Badge

type Tone = 'neutral' | 'accent' | 'ok' | 'warn' | 'bad' | 'info';

const toneClasses: Record<Tone, string> = {
  neutral: 'tw:bg-hover tw:text-muted tw:border-line',
  accent: 'tw:bg-accent-soft tw:text-accent tw:border-transparent',
  ok: 'tw:bg-ok-soft tw:text-ok tw:border-transparent',
  warn: 'tw:bg-warn-soft tw:text-warn tw:border-transparent',
  bad: 'tw:bg-bad-soft tw:text-bad tw:border-transparent',
  info: 'tw:bg-info-soft tw:text-info tw:border-transparent',
};

export function Badge({ tone = 'neutral', className, children, dot, ...rest }:
  React.HTMLAttributes<HTMLSpanElement> & { tone?: Tone; dot?: boolean }) {
  return (
    <span className={cn(
      'tw:inline-flex tw:items-center tw:gap-1 tw:rounded tw:border tw:px-1.5 tw:h-5 tw:text-2xs tw:font-medium tw:whitespace-nowrap',
      toneClasses[tone], className)} {...rest}>
      {dot && <span className="tw:size-1.5 tw:rounded-full tw:bg-current" />}
      {children}
    </span>
  );
}

export function StatusDot({ status, className }: { status: 'online' | 'recent' | 'offline'; className?: string }) {
  const color = status === 'online' ? 'tw:bg-ok' : status === 'recent' ? 'tw:bg-warn' : 'tw:bg-faint';
  return (
    <span className={cn('tw:relative tw:inline-flex tw:size-2 tw:shrink-0', className)}>
      {status === 'online' && <span className="tw:absolute tw:inset-0 tw:rounded-full tw:bg-ok tw:opacity-50 tw:animate-ping" />}
      <span className={cn('tw:relative tw:size-2 tw:rounded-full', color)} />
    </span>
  );
}

// ------------------------------------------------------------------ Card

export function Card({ className, ...rest }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('tw:bg-panel tw:border tw:border-line tw:rounded-lg rx-shadow', className)} {...rest} />;
}

export function CardHeader({ title, subtitle, actions, className, icon }: {
  title: React.ReactNode; subtitle?: React.ReactNode; actions?: React.ReactNode; className?: string; icon?: React.ReactNode;
}) {
  return (
    <div className={cn('tw:flex tw:items-center tw:gap-3 tw:px-4 tw:h-11 tw:border-b tw:border-line', className)}>
      {icon && <span className="tw:text-faint tw:flex tw:[&>svg:not([class*=size-])]:size-4">{icon}</span>}
      <div className="tw:min-w-0 tw:flex-1">
        <h3 className="tw:text-[13px] tw:font-semibold tw:truncate">{title}</h3>
        {subtitle && <div className="tw:text-2xs tw:text-faint tw:truncate">{subtitle}</div>}
      </div>
      {actions}
    </div>
  );
}

// ----------------------------------------------------------------- Input

export const Input = forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  function Input({ className, ...rest }, ref) {
    return (
      <input ref={ref}
        className={cn(
          'tw:h-8 tw:w-full tw:rounded-md tw:border tw:border-line-strong tw:bg-panel tw:px-2.5 tw:text-[13px]',
          'tw:placeholder:text-faint tw:outline-none tw:focus:border-accent tw:focus:ring-2 tw:focus:ring-accent/20',
          'tw:disabled:opacity-60',
          className)}
        {...rest} />
    );
  });

export const Textarea = forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  function Textarea({ className, ...rest }, ref) {
    return (
      <textarea ref={ref}
        className={cn(
          'tw:w-full tw:rounded-md tw:border tw:border-line-strong tw:bg-panel tw:px-2.5 tw:py-1.5 tw:text-[13px] tw:font-mono',
          'tw:placeholder:text-faint tw:outline-none tw:focus:border-accent tw:focus:ring-2 tw:focus:ring-accent/20',
          className)}
        {...rest} />
    );
  });

export function Select({ className, children, ...rest }: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={cn(
        'tw:h-8 tw:rounded-md tw:border tw:border-line-strong tw:bg-panel tw:px-2 tw:text-[13px] tw:outline-none',
        'tw:focus:border-accent tw:focus:ring-2 tw:focus:ring-accent/20',
        className)}
      {...rest}>
      {children}
    </select>
  );
}

export function SearchInput({ value, onChange, placeholder, className, autoFocus, inputRef, onKeyDown }: {
  value: string; onChange: (v: string) => void; placeholder?: string; className?: string; autoFocus?: boolean;
  inputRef?: React.Ref<HTMLInputElement>; onKeyDown?: React.KeyboardEventHandler<HTMLInputElement>;
}) {
  return (
    <div className={cn('tw:relative', className)}>
      <Search className="tw:absolute tw:left-2.5 tw:top-1/2 tw:-translate-y-1/2 tw:size-3.5 tw:text-faint tw:pointer-events-none" />
      <Input ref={inputRef} value={value} autoFocus={autoFocus} onKeyDown={onKeyDown}
        onChange={e => onChange(e.target.value)}
        placeholder={placeholder} className="tw:pl-8 tw:pr-7" />
      {value && (
        <button type="button" aria-label="Clear" onClick={() => onChange('')}
          className="tw:absolute tw:right-1.5 tw:top-1/2 tw:-translate-y-1/2 tw:p-0.5 tw:rounded tw:text-faint tw:hover:text-fg">
          <X className="tw:size-3.5" />
        </button>
      )}
    </div>
  );
}

export function Field({ label, hint, children, className, required }: {
  label: React.ReactNode; hint?: React.ReactNode; children: React.ReactNode; className?: string; required?: boolean;
}) {
  return (
    <label className={cn('tw:block tw:space-y-1', className)}>
      <span className="tw:block tw:text-xs tw:font-medium tw:text-fg">
        {label}{required && <span className="tw:text-bad"> *</span>}
      </span>
      {children}
      {hint && <span className="tw:block tw:text-2xs tw:text-faint">{hint}</span>}
    </label>
  );
}

export function Switch({ checked, onChange, label, disabled }: {
  checked: boolean; onChange: (v: boolean) => void; label?: React.ReactNode; disabled?: boolean;
}) {
  return (
    <label className={cn('tw:inline-flex tw:items-center tw:gap-2 tw:cursor-pointer tw:select-none', disabled && 'tw:opacity-50')}>
      <button type="button" role="switch" aria-checked={checked} disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cn('tw:relative tw:h-4 tw:w-7 tw:rounded-full tw:transition-colors',
          checked ? 'tw:bg-accent' : 'tw:bg-line-strong')}>
        <span className={cn('tw:absolute tw:top-0.5 tw:size-3 tw:rounded-full tw:bg-white tw:shadow tw:transition-all',
          checked ? 'tw:left-3.5' : 'tw:left-0.5')} />
      </button>
      {label && <span className="tw:text-[13px]">{label}</span>}
    </label>
  );
}

export function Checkbox({ checked, indeterminate, onChange, label, className }: {
  checked: boolean; indeterminate?: boolean; onChange: (v: boolean) => void; label?: string; className?: string;
}) {
  const ref = React.useRef<HTMLInputElement>(null);
  React.useEffect(() => {
    if (ref.current) ref.current.indeterminate = !!indeterminate;
  }, [indeterminate]);
  return (
    <input ref={ref} type="checkbox" aria-label={label} checked={checked}
      onChange={e => onChange(e.target.checked)}
      onClick={e => e.stopPropagation()}
      className={cn('tw:size-3.5 tw:rounded tw:cursor-pointer tw:accent-[var(--rx-accent)]', className)} />
  );
}

// -------------------------------------------------------------- Progress

export function Progress({ value, tone = 'accent', className, segments }: {
  value?: number; tone?: 'accent' | 'ok' | 'warn' | 'bad'; className?: string;
  segments?: { value: number; tone: 'accent' | 'ok' | 'warn' | 'bad' | 'muted' }[];
}) {
  const bg: Record<string, string> = {
    accent: 'tw:bg-accent', ok: 'tw:bg-ok', warn: 'tw:bg-warn', bad: 'tw:bg-bad', muted: 'tw:bg-faint',
  };
  return (
    <div className={cn('tw:h-1.5 tw:w-full tw:rounded-full tw:bg-hover tw:overflow-hidden tw:flex', className)}>
      {segments
        ? segments.map((s, i) => (
          <div key={i} className={cn('tw:h-full tw:transition-all', bg[s.tone])} style={{ width: `${s.value}%` }} />))
        : <div className={cn('tw:h-full tw:rounded-full tw:transition-all', bg[tone])} style={{ width: `${value ?? 0}%` }} />}
    </div>
  );
}

// --------------------------------------------------------------- Loading

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn('tw:size-4 tw:animate-spin tw:text-faint', className)} />;
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('rx-skeleton tw:h-3', className)} />;
}

export function EmptyState({ icon, title, children, action, className }: {
  icon?: React.ReactNode; title: React.ReactNode; children?: React.ReactNode; action?: React.ReactNode; className?: string;
}) {
  return (
    <div className={cn('tw:flex tw:flex-col tw:items-center tw:justify-center tw:text-center tw:gap-2 tw:p-10', className)}>
      {icon && <div className="tw:text-faint tw:mb-1 tw:[&>svg]:size-8">{icon}</div>}
      <div className="tw:text-sm tw:font-semibold">{title}</div>
      {children && <div className="tw:text-xs tw:text-muted tw:max-w-sm">{children}</div>}
      {action && <div className="tw:mt-2">{action}</div>}
    </div>
  );
}

export function Kbd({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <kbd className={cn(
      'tw:inline-flex tw:items-center tw:h-[18px] tw:min-w-[18px] tw:justify-center tw:rounded tw:border tw:border-line-strong',
      'tw:bg-panel tw:px-1 tw:text-2xs tw:font-sans tw:text-muted', className)}>{children}</kbd>
  );
}

// --------------------------------------------------------------- Tooltip

export function Tooltip({ content, children, side = 'top' }: {
  content: React.ReactNode; children: React.ReactElement; side?: 'top' | 'bottom' | 'left' | 'right';
}) {
  if (!content) return children;
  return (
    <RTooltip.Root delayDuration={300}>
      <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
      <RTooltip.Portal>
        <RTooltip.Content side={side} sideOffset={6}
          className="rx tw:z-[2000] tw:rounded-md tw:bg-fg tw:text-canvas tw:px-2 tw:py-1 tw:text-xs tw:shadow-lg tw:animate-fade-in tw:max-w-xs">
          {content}
        </RTooltip.Content>
      </RTooltip.Portal>
    </RTooltip.Root>
  );
}

export const TooltipProvider = RTooltip.Provider;

// ---------------------------------------------------------------- Dialog

export function Dialog({ open, onOpenChange, title, description, children, footer, width = 560, className }: {
  open: boolean; onOpenChange: (v: boolean) => void; title: React.ReactNode; description?: React.ReactNode;
  children: React.ReactNode; footer?: React.ReactNode; width?: number | string; className?: string;
}) {
  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className="rx tw:fixed tw:inset-0 tw:z-[1500] tw:bg-black/40 tw:backdrop-blur-[1px] tw:animate-fade-in" />
        <RDialog.Content
          aria-describedby={undefined}
          style={{ width, maxWidth: 'calc(100vw - 32px)' }}
          className={cn(
            'rx tw:fixed tw:left-1/2 tw:top-1/2 tw:z-[1501] tw:-translate-x-1/2 tw:-translate-y-1/2',
            'tw:max-h-[calc(100vh-48px)] tw:flex tw:flex-col tw:bg-panel tw:rounded-xl tw:border tw:border-line rx-shadow-lg tw:animate-pop-in',
            'tw:outline-none',
            className)}>
          <div className="tw:flex tw:items-start tw:gap-3 tw:px-5 tw:pt-4 tw:pb-3">
            <div className="tw:flex-1 tw:min-w-0">
              <RDialog.Title className="tw:text-[15px] tw:font-semibold">{title}</RDialog.Title>
              {description && <RDialog.Description className="tw:text-xs tw:text-muted tw:mt-0.5">{description}</RDialog.Description>}
            </div>
            <RDialog.Close className="tw:p-1 tw:rounded tw:text-faint tw:hover:bg-hover tw:hover:text-fg" aria-label="Close">
              <X className="tw:size-4" />
            </RDialog.Close>
          </div>
          <div className="tw:flex-1 tw:overflow-auto tw:px-5 tw:pb-4">{children}</div>
          {footer && (
            <div className="tw:flex tw:items-center tw:justify-end tw:gap-2 tw:px-5 tw:py-3 tw:border-t tw:border-line tw:bg-canvas/50 tw:rounded-b-xl">
              {footer}
            </div>
          )}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

export function ConfirmDialog({ open, onOpenChange, title, children, confirmLabel, danger, onConfirm, loading }: {
  open: boolean; onOpenChange: (v: boolean) => void; title: React.ReactNode; children?: React.ReactNode;
  confirmLabel: string; danger?: boolean; onConfirm: () => void; loading?: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={title} width={440}
      footer={<>
        <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
        <Button variant={danger ? 'danger' : 'primary'} loading={loading} onClick={onConfirm} autoFocus>{confirmLabel}</Button>
      </>}>
      <div className="tw:text-[13px] tw:text-muted">{children}</div>
    </Dialog>
  );
}

// ------------------------------------------------------------------ Menu

export const Menu = RMenu.Root;
export const MenuTrigger = RMenu.Trigger;

export function MenuContent({ children, align = 'end', className }: {
  children: React.ReactNode; align?: 'start' | 'end' | 'center'; className?: string;
}) {
  return (
    <RMenu.Portal>
      <RMenu.Content align={align} sideOffset={4}
        className={cn('rx tw:z-[1600] tw:min-w-[180px] tw:rounded-lg tw:border tw:border-line tw:bg-raised tw:p-1 rx-shadow-lg tw:animate-fade-in', className)}>
        {children}
      </RMenu.Content>
    </RMenu.Portal>
  );
}

export function MenuItem({ children, icon, onSelect, danger, disabled, shortcut }: {
  children: React.ReactNode; icon?: React.ReactNode; onSelect?: () => void; danger?: boolean; disabled?: boolean; shortcut?: string;
}) {
  return (
    <RMenu.Item disabled={disabled} onSelect={onSelect}
      className={cn(
        'tw:flex tw:items-center tw:gap-2 tw:h-8 tw:px-2 tw:rounded-md tw:text-[13px] tw:outline-none tw:cursor-pointer',
        'tw:data-[highlighted]:bg-hover tw:data-[disabled]:opacity-40 tw:data-[disabled]:cursor-default',
        danger ? 'tw:text-bad' : 'tw:text-fg')}>
      {icon && <span className="tw:text-faint tw:[&>svg]:size-3.5 tw:flex">{icon}</span>}
      <span className="tw:flex-1">{children}</span>
      {shortcut && <span className="tw:text-2xs tw:text-faint">{shortcut}</span>}
    </RMenu.Item>
  );
}

export function MenuSeparator() {
  return <RMenu.Separator className="tw:my-1 tw:h-px tw:bg-line" />;
}

export function MenuLabel({ children }: { children: React.ReactNode }) {
  return <RMenu.Label className="tw:px-2 tw:pt-1.5 tw:pb-1 tw:text-2xs tw:font-semibold tw:uppercase tw:tracking-wide tw:text-faint">{children}</RMenu.Label>;
}

// ------------------------------------------------------------------ Tabs

export function Tabs({ value, onValueChange, tabs, className, right }: {
  value: string; onValueChange: (v: string) => void; className?: string; right?: React.ReactNode;
  tabs: { value: string; label: React.ReactNode; count?: React.ReactNode; icon?: React.ReactNode }[];
}) {
  return (
    <RTabs.Root value={value} onValueChange={onValueChange}>
      <div className={cn('tw:flex tw:items-end tw:gap-4 tw:border-b tw:border-line tw:px-4', className)}>
        <RTabs.List className="tw:flex tw:gap-1 tw:-mb-px tw:overflow-x-auto">
          {tabs.map(t => (
            <RTabs.Trigger key={t.value} value={t.value}
              className={cn(
                'tw:flex tw:items-center tw:gap-1.5 tw:h-9 tw:px-2.5 tw:text-[13px] tw:text-muted tw:border-b-2 tw:border-transparent tw:whitespace-nowrap',
                'tw:hover:text-fg tw:data-[state=active]:text-fg tw:data-[state=active]:border-accent tw:font-medium')}>
              {t.icon && <span className="tw:[&>svg]:size-3.5 tw:flex">{t.icon}</span>}
              {t.label}
              {t.count !== undefined && (
                <span className="tw:rounded tw:bg-hover tw:px-1.5 tw:text-2xs tw:text-muted rx-num">{t.count}</span>
              )}
            </RTabs.Trigger>
          ))}
        </RTabs.List>
        {right && <div className="tw:ml-auto tw:flex tw:items-center tw:gap-2 tw:pb-1.5">{right}</div>}
      </div>
    </RTabs.Root>
  );
}

// ---------------------------------------------------------- Segmented

export function Segmented<T extends string>({ value, onChange, options, size = 'sm' }: {
  value: T; onChange: (v: T) => void; options: { value: T; label: React.ReactNode; count?: number }[]; size?: 'sm' | 'md';
}) {
  return (
    <div className="tw:inline-flex tw:rounded-md tw:bg-hover tw:p-0.5 tw:gap-0.5">
      {options.map(o => (
        <button key={o.value} type="button" onClick={() => onChange(o.value)}
          className={cn(
            'tw:rounded tw:px-2.5 tw:font-medium tw:flex tw:items-center tw:gap-1.5 tw:transition-colors',
            size === 'sm' ? 'tw:h-6 tw:text-xs' : 'tw:h-7 tw:text-[13px]',
            o.value === value ? 'tw:bg-panel tw:text-fg tw:shadow-sm' : 'tw:text-muted tw:hover:text-fg')}>
          {o.label}
          {o.count !== undefined && <span className="tw:text-2xs tw:text-faint rx-num">{o.count}</span>}
        </button>
      ))}
    </div>
  );
}

// ------------------------------------------------------------- Misc

export function Stat({ label, value, sub, icon, tone, onClick }: {
  label: React.ReactNode; value: React.ReactNode; sub?: React.ReactNode; icon?: React.ReactNode;
  tone?: 'accent' | 'ok' | 'warn' | 'bad' | 'info'; onClick?: () => void;
}) {
  const toneBg: Record<string, string> = {
    accent: 'tw:bg-accent-soft tw:text-accent', ok: 'tw:bg-ok-soft tw:text-ok',
    warn: 'tw:bg-warn-soft tw:text-warn', bad: 'tw:bg-bad-soft tw:text-bad', info: 'tw:bg-info-soft tw:text-info',
  };
  const Comp = onClick ? 'button' : 'div';
  return (
    <Card className={cn('tw:p-4 tw:text-left', onClick && 'tw:hover:border-line-strong tw:transition-colors')}>
      <Comp onClick={onClick} className="tw:w-full tw:text-left tw:flex tw:items-start tw:gap-3">
        {icon && (
          <span className={cn('tw:flex tw:size-9 tw:shrink-0 tw:items-center tw:justify-center tw:rounded-lg tw:[&>svg]:size-[18px]',
            toneBg[tone ?? 'accent'])}>{icon}</span>
        )}
        <div className="tw:min-w-0">
          <div className="tw:text-xs tw:text-muted">{label}</div>
          <div className="tw:text-2xl tw:font-semibold tw:tracking-tight rx-num tw:leading-tight">{value}</div>
          {sub && <div className="tw:text-2xs tw:text-faint tw:mt-0.5">{sub}</div>}
        </div>
      </Comp>
    </Card>
  );
}

export function Mono({ children, className }: { children: React.ReactNode; className?: string }) {
  return <span className={cn('tw:font-mono tw:text-[12px]', className)}>{children}</span>;
}

export function CopyButton({ text, className }: { text: string; className?: string }) {
  const [copied, setCopied] = React.useState(false);
  return (
    <button type="button"
      onClick={e => {
        e.stopPropagation();
        navigator.clipboard?.writeText(text).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1200);
        });
      }}
      className={cn('tw:text-2xs tw:text-faint tw:hover:text-fg tw:px-1 tw:rounded', className)}>
      {copied ? 'Copied' : 'Copy'}
    </button>
  );
}
