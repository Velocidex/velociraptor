// Renders trusted-but-sanitized Markdown (artifact descriptions etc).
import { useMemo } from 'react';
import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';
import { cn } from './primitives';

const md = new MarkdownIt({ html: false, linkify: true, breaks: false });

export default function Markdown({ text, className }: { text: string; className?: string }) {
  const html = useMemo(() => DOMPurify.sanitize(md.render(text ?? '')), [text]);
  return <div className={cn('rx-md', className)} dangerouslySetInnerHTML={{ __html: html }} />;
}
