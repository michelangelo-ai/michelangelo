export type TextEditorProps = {
  value: string;
  language?: 'json';
  readOnly?: boolean;
  height?: string;
  onChange?: (value: string) => void;
};
