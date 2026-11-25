import type { NotebookCell } from '../../lib/queries';
export default function LegacyCellOutput(props: {
  cell: NotebookCell; notebookId: string; env?: Record<string, string>; refresh?: () => void;
}): JSX.Element;
