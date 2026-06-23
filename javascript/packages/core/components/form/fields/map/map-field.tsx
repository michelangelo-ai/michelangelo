import { useCallback, useRef, useState } from 'react';
import { useStyletron } from 'baseui';

import { AddButton } from '#core/components/form/components/add-button/add-button';
import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { KeyValueRow } from './key-value-row';

import type { KeyValueEntry, MapFieldProps } from './types';

const EMPTY_RECORD: Record<string, string> = {};
const DUPLICATE_KEY_ERROR = 'cannot have duplicated values';

export function MapField({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  readOnly,
  disabled,
  description,
  caption,
  labelEndEnhancer,
  format,
  parse,
  keyConfig,
  valueConfig,
  singleValue,
  creatable = true,
  deletable = true,
  emptyMessage,
  size,
}: MapFieldProps) {
  const [css, theme] = useStyletron();
  const { input, meta } = useField<Record<string, string>>(name, {
    required,
    validate,
    defaultValue: defaultValue ?? EMPTY_RECORD,
    initialValue,
    label,
    format,
    parse,
  });

  const rowIdCounter = useRef(0);

  // Rows carry a stable 'id' for React keys that doesn't exist in the form value.
  const [rows, setRows] = useState<KeyValueEntry[]>(() => {
    const initial = toRows(input.value);
    rowIdCounter.current = initial.length;
    if (singleValue && initial.length === 0) {
      rowIdCounter.current = 1;
      return [{ id: 0, key: '', value: '' }];
    }
    return initial;
  });

  const handleRowChange = useCallback(
    (updated: KeyValueEntry) => {
      setRows((prev) => {
        const next = prev.map((r) => (r.id === updated.id ? updated : r));
        input.onChange(toRecord(next));
        return next;
      });
    },
    [input]
  );

  // Empty rows don't affect form state until the user types a key or value.
  const handleAdd = () => {
    setRows((prev) => [...prev, { id: rowIdCounter.current++, key: '', value: '' }]);
  };

  const handleRemove = useCallback(
    (row: KeyValueEntry) => {
      setRows((prev) => {
        const next = prev.filter((r) => r.id !== row.id);
        input.onChange(toRecord(next));
        return next;
      });
    },
    [input]
  );

  const duplicateKeys = new Set(
    rows.map((r) => r.key).filter((k, i, arr) => k && arr.indexOf(k) !== i)
  );

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      labelEndEnhancer={labelEndEnhancer}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      <div
        className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale300 })}
      >
        {rows.length === 0 && emptyMessage && (
          <div
            className={css({
              ...theme.typography.ParagraphSmall,
              marginBottom: theme.sizing.scale200,
            })}
          >
            {emptyMessage}
          </div>
        )}

        {rows.map((row) => (
          <KeyValueRow
            key={row.id}
            row={row}
            keyConfig={keyConfig}
            valueConfig={valueConfig}
            readOnly={readOnly}
            disabled={disabled}
            deletable={deletable && !singleValue}
            size={size}
            keyError={
              duplicateKeys.has(row.key)
                ? `${keyConfig?.placeholder ?? 'Keys'} ${DUPLICATE_KEY_ERROR}`
                : undefined
            }
            onChange={handleRowChange}
            onDelete={handleRemove}
            onFocus={input.onFocus}
            onBlur={input.onBlur}
          />
        ))}

        {!singleValue && !readOnly && creatable && (
          <AddButton onClick={handleAdd} label="Add more" />
        )}
      </div>
    </FormControl>
  );
}

function toRows(value: Record<string, string> | undefined): KeyValueEntry[] {
  return Object.entries(value ?? {}).map(([key, val], i) => ({ id: i, key, value: val }));
}

function toRecord(rows: KeyValueEntry[]): Record<string, string> {
  return Object.fromEntries(rows.map((r) => [r.key, r.value]));
}
