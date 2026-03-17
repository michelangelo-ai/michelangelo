import { useStyletron } from 'baseui';
import { Button, KIND, SIZE } from 'baseui/button';

import { Icon } from '#core/components/icon/icon';

import type { AddButtonProps } from './types';

/**
 * Styled add button for form array layouts.
 *
 * Provides a consistent "add item" action across array-based form components.
 * Renders a secondary compact button with a plus icon.
 *
 * @param props.label - Button label. Defaults to "Add more".
 * @param props.shape - BaseUI button shape variant.
 * @param props.onClick - Callback fired when the button is clicked.
 *
 * @example
 * ```tsx
 * const { add } = useArrayField('items');
 *
 * <AddButton onClick={add} label="Add item" />
 * ```
 */
export function AddButton({ label = 'Add more', shape, onClick }: AddButtonProps) {
  const [, theme] = useStyletron();

  return (
    <Button
      type="button"
      kind={KIND.secondary}
      size={SIZE.compact}
      shape={shape}
      startEnhancer={
        <Icon name="plus" color={theme.colors.contentPrimary} size={theme.sizing.scale600} />
      }
      overrides={{
        BaseButton: { style: { marginBottom: theme.sizing.scale600, width: '260px' } },
      }}
      onClick={onClick}
    >
      {label}
    </Button>
  );
}
