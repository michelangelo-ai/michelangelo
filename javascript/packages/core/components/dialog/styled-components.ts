import type { Theme } from 'baseui';
import type { DialogProps } from 'baseui/dialog';

export const LAYER_HEADER_ABOVE_CONTENTS: NonNullable<DialogProps['overrides']> = {
  DismissButton: { props: { overrides: { BaseButton: { style: { zIndex: 2 } } } } },
  Heading: { style: { zIndex: 1 } },
};

export function enableButtonDockShadow(hasScrolledToBottom: boolean) {
  return {
    ButtonDock: {
      props: {
        overrides: {
          Root: {
            style: ({ $theme }: { $theme: Theme }) => ({
              boxShadow: hasScrolledToBottom ? 'none' : $theme.lighting.shallowAbove,
              transition: `box-shadow ${$theme.animation.timing500} ${$theme.animation.easeOutQuinticCurve}`,
            }),
          },
        },
      },
    },
  };
}
