import type { SIZE } from 'baseui/input';
import type { BaseFieldProps } from '#core/components/form/fields/types';

export interface KeyValueEntry {
  /** Stable React key — monotonically increasing, never reused after deletion. */
  id: number;
  key: string;
  value: string;
}

/** Props shared between MapField and KeyValueRow — controls how each row renders. */
export interface KeyValueRowConfig {
  /** Configuration for the key input column */
  keyConfig?: { placeholder?: string; readOnly?: boolean };

  /** Configuration for the value input column */
  valueConfig?: { placeholder?: string };

  /** Show delete button per row. Defaults to true. */
  deletable?: boolean;

  /** BaseUI input size variant */
  size?: keyof typeof SIZE;
}

export interface MapFieldOwnProps extends KeyValueRowConfig {
  /** When true, renders exactly one key-value pair with no add/delete controls */
  singleValue?: boolean;

  /** Show "Add more" button. Defaults to true. */
  creatable?: boolean;

  /** Message shown when the map is empty */
  emptyMessage?: string;
}

export type MapFieldProps = MapFieldOwnProps & BaseFieldProps<Record<string, string>>;
