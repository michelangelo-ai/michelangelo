import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE } from 'baseui/button';
import { Input } from 'baseui/input';

import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';

import type { KeyValueEntry, KeyValueRowConfig } from './types';

interface KeyValueRowProps extends KeyValueRowConfig {
  row: KeyValueEntry;
  readOnly?: boolean;
  disabled?: boolean;
  keyError?: string;
  onChange: (row: KeyValueEntry) => void;
  onDelete: (row: KeyValueEntry) => void;
  onFocus: () => void;
  onBlur: () => void;
}

export function KeyValueRow({
  row,
  keyConfig,
  valueConfig,
  readOnly,
  disabled,
  deletable = true,
  size,
  keyError,
  onChange,
  onDelete,
  onFocus,
  onBlur,
}: KeyValueRowProps) {
  const [css, theme] = useStyletron();

  const blurIfRowFilled = row.key && row.value ? onBlur : undefined;

  return (
    <div
      className={css({
        display: 'flex',
        alignItems: 'flex-start',
        gap: theme.sizing.scale300,
        width: '100%',
      })}
    >
      <Input
        value={row.key}
        onChange={(e) => onChange({ ...row, key: e.currentTarget.value })}
        onFocus={onFocus}
        onBlur={blurIfRowFilled}
        placeholder={keyConfig?.placeholder ?? 'Key'}
        // eslint-disable-next-line @typescript-eslint/prefer-nullish-coalescing -- false is a valid value, so || is needed to fall through to keyConfig?.readOnly
        readOnly={readOnly || keyConfig?.readOnly}
        disabled={disabled}
        size={size}
        error={!!keyError}
        overrides={{
          Root: {
            style: { flex: 1 },
            props: keyError ? { title: keyError } : undefined,
          },
        }}
      />
      <Input
        value={row.value}
        onChange={(e) => onChange({ ...row, value: e.currentTarget.value })}
        onFocus={onFocus}
        onBlur={blurIfRowFilled}
        placeholder={valueConfig?.placeholder ?? 'Value'}
        readOnly={readOnly}
        disabled={disabled}
        size={size}
        overrides={{ Root: { style: { flex: 3 } } }}
      />
      {!readOnly && deletable && (
        <Button
          type="button"
          aria-label="Delete"
          kind={KIND.tertiary}
          shape={SHAPE.circle}
          onClick={() => onDelete(row)}
          overrides={{
            BaseButton: {
              style: { marginBottom: theme.sizing.scale600 },
            },
          }}
        >
          <Icon name="trashCan" kind={IconKind.TERTIARY} />
        </Button>
      )}
    </div>
  );
}
