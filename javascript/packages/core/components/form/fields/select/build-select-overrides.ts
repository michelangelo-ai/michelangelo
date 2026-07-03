import { mergeOverrides } from 'baseui';

import type { Theme } from 'baseui';
import type { SelectOverrides } from 'baseui/select';

export function buildSelectOverrides(
  name: string,
  disabled?: boolean,
  readOnly?: boolean
): SelectOverrides {
  // When rendered inside modals, dropdown options can overflow past the modal body into its backdrop.
  // popperOptions ensures preventOverflow is registered before the hide modifier (Popper.js v1
  // requires this order), so ignoreBoundary doesn't get its overflow-prevention undone by hide.
  // name is forwarded to the internal <input> so form-focus libraries (e.g. final-form-focus) can
  // match this control to its field error by name.
  const base = {
    Popover: {
      props: {
        ignoreBoundary: true,
        popperOptions: {
          modifiers: {
            preventOverflow: { enabled: true, boundariesElement: 'window', padding: 8 },
          },
        },
      },
    },
    Input: { props: { name } },
  };

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
