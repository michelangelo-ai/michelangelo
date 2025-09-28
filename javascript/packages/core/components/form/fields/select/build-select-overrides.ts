import { mergeOverrides } from 'baseui';

import type { Theme } from 'baseui';
import type { SelectOverrides } from 'baseui/select';

export function buildSelectOverrides(disabled?: boolean, readOnly?: boolean): SelectOverrides {
  // When rendered inside modals, dropdown options can overflow past the modal body into its backdrop.
  const base = { Popover: { props: { ignoreBoundary: true } } };

  if (disabled) {
    return mergeOverrides(base, { Tag: { props: { closeable: false, disabled: false } } });
  }

  if (readOnly) {
    return mergeOverrides(base, {
      Input: { props: { readOnly: true } },
      ControlContainer: {
        props: { onClick: () => null },
        style: ({ $theme }: { $theme: Theme }) => ({
          backgroundColor: $theme.colors.backgroundPrimary,
        }),
      },
      SelectArrow: { props: { size: 0 } },
      Tag: { props: { closeable: false } },
    });
  }

  return base;
}
