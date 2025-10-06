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

/**
 * Baseweb's button dock leverages container queries to styling of the button dock
 * for dialogs, based on the size of the dialog. These container queries do not appear
 * to work with the version of Styletron we are using.
 *
 * To match the expected behavior, we tag the button dock with a data-baseweb attribute
 * and add a container query using this attribute to packages/core/styles/main.css stylesheet
 */
export const ENABLE_BUTTON_DOCK_CONTAINER_QUERY_WORKAROUND = {
  ButtonDock: {
    props: {
      overrides: {
        Root: {
          props: {
            'data-baseweb': 'button-dock',
          },
        },
      },
    },
  },
};
