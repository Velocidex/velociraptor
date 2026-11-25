// Charts produced by notebook templates ({{ Query "x" | LineChart }} and
// friends), drawn with the same look as the dashboard. The first column
// is the X axis, every numeric column after it becomes a series.
import { useMemo, useState } from 'react';
import {
  Bar, BarChart, CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Scatter, ScatterChart, Tooltip,
  XAxis, YAxis,
} from 'recharts';
import { useQuery } from '@tanstack/react-query';
import { RotateCcw } from 'lucide-react';
import * as api from '../../lib/api';
import { parseTable, type Row } from '../../lib/table';
import { fmtCompact, fmtTime, toDate } from '../../lib/format';
import type { TableResponse } from '../../lib/types';
import { Button, Skeleton } from '../../ui/primitives';

export type ChartKind = 'line' | 'time' | 'bar' | 'scatter';

// Distinct in both themes; the first matches the accent.
const SERIES = ['var(--rx-accent)', 'var(--rx-info)', 'var(--rx-ok)', 'var(--rx-warn)', 'var(--rx-bad)', '#a855f7', '#14b8a6', '#f97316'];

const axisTick = { fontSize: 10, fill: 'var(--rx-faint)' };
const tooltipStyle = { background: 'var(--rx-raised)', border: '1px solid var(--rx-line)', borderRadius: 8, fontSize: 12 };

function toNumber(v: unknown): number | null {
  if (typeof v === 'number') return v;
  if (typeof v === 'string' && v.trim() !== '' && !Number.isNaN(Number(v))) return Number(v);
  return null;
}

export default function NotebookChart({ kind, params }: { kind: ChartKind; params: Record<string, unknown> }) {
  const q = useQuery({
    queryKey: ['notebook-chart', params],
    queryFn: ({ signal }) => api.get<TableResponse>('v1/GetTable', { rows: 10000, ...params, TableOptions: undefined }, signal),
    select: parseTable,
    staleTime: 5 * 60_000,
  });
  const [zoom, setZoom] = useState<[number, number] | null>(null);
  const [drag, setDrag] = useState<[number, number | null] | null>(null);

  const { data, xKey, series } = useMemo(() => {
    const cols = q.data?.columns ?? [];
    const rows = q.data?.rows ?? [];
    const xKey = cols[0] ?? '';
    const series = cols.slice(1).filter(c => !c.startsWith('_') && rows.some(r => toNumber(r[c]) !== null));
    const data = rows.map(r => {
      const o: Row = {};
      if (kind === 'time') o[xKey] = toDate(r[xKey] as number | string)?.getTime() ?? 0;
      else if (kind === 'bar') o[xKey] = String(r[xKey] ?? '');
      else o[xKey] = toNumber(r[xKey]) ?? 0;
      for (const s of series) o[s] = toNumber(r[s]);
      return o;
    });
    return { data, xKey, series };
  }, [q.data, kind]);

  if (q.isLoading) return <Skeleton className="tw:h-72 tw:my-2" />;
  if (q.error) return <div className="tw:text-xs tw:text-bad tw:py-2">Could not load chart: {(q.error as Error).message}</div>;
  if (!data.length || !series.length) {
    return <div className="tw:text-xs tw:text-faint tw:py-6 tw:text-center">No numeric data to chart.</div>;
  }

  const numericX = kind === 'time' || kind === 'line' || kind === 'scatter';
  const shown = zoom && numericX ? data.filter(d => (d[xKey] as number) >= zoom[0] && (d[xKey] as number) <= zoom[1]) : data;
  const xFmt = kind === 'time' ? (v: number) => fmtTime(v).replace(/^\w+ \d+, \d+, /, '') : (v: number | string) => (typeof v === 'number' ? fmtCompact(v) : v);
  const labelFmt = kind === 'time' ? (v: number) => fmtTime(v) : undefined;
  const common = {
    data: shown,
    margin: { top: 8, right: 16, bottom: 0, left: 0 },
  };
  const axes = (
    <>
      <CartesianGrid stroke="var(--rx-line)" vertical={false} />
      <XAxis dataKey={xKey} type={numericX ? 'number' : 'category'} domain={numericX ? ['dataMin', 'dataMax'] : undefined}
        tickFormatter={xFmt as (v: unknown) => string} tick={axisTick} axisLine={false} tickLine={false} minTickGap={40}
        allowDataOverflow />
      <YAxis dataKey={kind === 'scatter' ? '__y' : undefined} type="number" tick={axisTick} axisLine={false} tickLine={false} tickFormatter={v => fmtCompact(v)} width={48} />
      <Tooltip contentStyle={tooltipStyle} labelFormatter={labelFmt as ((l: unknown) => string) | undefined}
        cursor={{ stroke: 'var(--rx-line-strong)' }} />
      {series.length > 1 && <Legend wrapperStyle={{ fontSize: 11 }} iconType="plainline" />}
    </>
  );

  // Drag across a line/time chart to zoom; double click to reset.
  const zoomHandlers = numericX && kind !== 'scatter' ? {
    onMouseDown: (e: { activeLabel?: number | string } | null) => e?.activeLabel !== undefined && setDrag([Number(e.activeLabel), null]),
    onMouseMove: (e: { activeLabel?: number | string } | null) => drag && e?.activeLabel !== undefined && setDrag([drag[0], Number(e.activeLabel)]),
    onMouseUp: () => {
      if (drag && drag[1] !== null && drag[0] !== drag[1]) setZoom([Math.min(drag[0], drag[1]), Math.max(drag[0], drag[1])]);
      setDrag(null);
    },
  } : {};

  return (
    <div className="tw:relative tw:my-2 tw:select-none" onDoubleClick={() => setZoom(null)} onClick={e => e.stopPropagation()}>
      {zoom && (
        <Button size="xs" variant="secondary" icon={<RotateCcw />} className="tw:absolute tw:right-2 tw:top-0 tw:z-10"
          onClick={() => setZoom(null)}>Reset zoom</Button>
      )}
      <ResponsiveContainer width="100%" height={320}>
        {kind === 'bar' ? (
          <BarChart {...common}>
            {axes}
            {series.map((s, i) => (
              <Bar key={s} dataKey={s} maxBarSize={56} fill={SERIES[i % SERIES.length]} radius={[3, 3, 0, 0]} isAnimationActive={false}
                stackId={params.type === 'stacked' ? 'a' : undefined} />
            ))}
          </BarChart>
        ) : kind === 'scatter' ? (
          <ScatterChart {...common}>
            {axes}
            {series.map((s, i) => (
              <Scatter key={s} name={s} data={shown.map(d => ({ [xKey]: d[xKey], __y: d[s] }))}
                fill={SERIES[i % SERIES.length]} isAnimationActive={false} />
            ))}
          </ScatterChart>
        ) : (
          <LineChart {...common} {...zoomHandlers}>
            {axes}
            {series.map((s, i) => (
              <Line key={s} type="monotone" dataKey={s} stroke={SERIES[i % SERIES.length]} strokeWidth={1.5}
                dot={shown.length < 40} isAnimationActive={false} connectNulls />
            ))}
          </LineChart>
        )}
      </ResponsiveContainer>
      {drag && drag[1] !== null && <div className="tw:text-2xs tw:text-faint tw:text-center">Release to zoom</div>}
      {!zoom && numericX && kind !== 'scatter' && shown.length > 20 && (
        <div className="tw:text-2xs tw:text-faint tw:text-center">Drag across the chart to zoom, double click to reset</div>
      )}
    </div>
  );
}
