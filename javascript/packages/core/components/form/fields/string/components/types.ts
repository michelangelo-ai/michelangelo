import type { SharedProps as SharedInputProps } from 'baseui/input';

export interface StringTagInputProps extends SharedInputProps {
  clear: () => void;
  readOnly?: boolean;
  removeValue: (index: number) => void;
  updateValue: (newValue: string, index: number) => void;
  value?: string | number;
  valueList: string[];
}

export interface EditableStringTagProps {
  closeable: boolean;
  index: number;
  onRemove: () => void;
  readOnly?: boolean;
  updateValue: (newValue: string, index: number) => void;
  value: string;
}
