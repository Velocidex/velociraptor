declare const CodeEditor: (props: {
  value: string;
  onChange?: (v: string) => void;
  mode?: 'vql' | 'yaml' | 'markdown' | 'text' | 'regex' | 'yara' | 'sigma';
  readOnly?: boolean;
  minLines?: number;
  maxLines?: number;
  className?: string;
  focus?: boolean;
}) => JSX.Element;
export default CodeEditor;
