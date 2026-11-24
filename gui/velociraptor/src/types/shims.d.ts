declare module 'qs';
declare module '*.svg' {
  const src: string;
  export default src;
}

declare module 'markdown-it' {
  export default class MarkdownIt {
    constructor(opts?: Record<string, unknown>);
    render(src: string): string;
  }
}
