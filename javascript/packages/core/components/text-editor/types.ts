export type TextEditorProps = {
  value: string;
  language?: 'json';
  readOnly?: boolean;
  height?: string;
  /**
   * Show controls in the gutter for collapsing/expanding nested blocks
   * (objects, arrays), useful for navigating large documents. Defaults to false.
   */
  foldable?: boolean;
  onChange?: (value: string) => void;
};
