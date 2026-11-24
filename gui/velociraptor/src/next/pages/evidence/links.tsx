import { Link } from 'react-router-dom';

export function ClientLink({ id, hostname }: { id: string; hostname?: string }) {
  if (!id) return null;
  return (
    <Link to={`/host/${id}`} onClick={e => e.stopPropagation()}
      className="tw:font-mono tw:text-[12px] tw:text-accent tw:hover:underline">{hostname || id}</Link>
  );
}

export function FlowLink({ clientId, flowId, children }: { clientId: string; flowId: string; children?: React.ReactNode }) {
  if (!flowId) return null;
  return (
    <Link to={`/collected/${clientId}/${flowId}`} onClick={e => e.stopPropagation()}
      className="tw:font-mono tw:text-[12px] tw:text-accent tw:hover:underline">{children ?? flowId}</Link>
  );
}
