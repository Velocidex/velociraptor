// On phones the list/detail pages show one pane at a time (see .rx-split
// in styles.css). This bar takes the person back to the list.
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronLeft } from 'lucide-react';

export function MobileBack({ to, label }: { to: string; label: string }) {
  return (
    <Link to={to} className="tw:md:hidden tw:flex tw:items-center tw:gap-1 tw:h-10 tw:px-3 tw:shrink-0 tw:border-b tw:border-line tw:text-xs tw:text-muted tw:hover:text-fg tw:bg-panel">
      <ChevronLeft className="tw:size-4" />{label}
    </Link>
  );
}

export const isPhone = () => typeof window !== 'undefined' && window.matchMedia?.('(max-width: 767.98px)').matches;

export function usePhone(): boolean {
  const [phone, setPhone] = useState(isPhone);
  useEffect(() => {
    const mq = window.matchMedia?.('(max-width: 767.98px)');
    if (!mq) return;
    const on = () => setPhone(mq.matches);
    mq.addEventListener('change', on);
    return () => mq.removeEventListener('change', on);
  }, []);
  return !!phone;
}
