export type TextEditorProps = {
  value: string;
  language?: 'json';
  readOnly?: boolean;
  height?: string;
  foldGutter?: boolean;
  onChange?: (value: string) => void;
};
