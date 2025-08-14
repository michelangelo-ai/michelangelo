import { styled } from 'baseui';
import { StatefulPanel } from 'baseui/accordion';

import { StyledBoxContainer } from '#core/components/box/styled-components';

import type { Theme } from 'baseui';
import type { StatefulPanelProps } from 'baseui/accordion';

export const TaskSeparator = styled('hr', ({ $theme }) => ({
  border: 'none',
  borderBottom: `2px dashed ${$theme.colors.contentInverseTertiary}`,
  margin: `${$theme.sizing.scale200} 0`,
}));

/**
 * Standard vertical stack layout for organizing task-related content.
 * Provides consistent spacing between task components, sections, and lists.
 */
export const TaskContentStack = styled('div', ({ $theme }) => ({
  display: 'flex',
  flexDirection: 'column',
  gap: $theme.sizing.scale800,
}));

export function TaskPanel(props: StatefulPanelProps & { id?: string }) {
  const { id, ...restProps } = props;
  return (
    <StatefulPanel
      {...restProps}
      overrides={{
        PanelContainer: {
          component: StyledBoxContainer,
          props: {
            id,
            onClick: (e: MouseEvent) => e.stopPropagation(),
          },
          style: ({ $theme, $expanded }: { $expanded: boolean; $theme: Theme }) => ({
            ...(!$expanded && { gap: 0 }),
            transitionProperty: 'gap',
            transitionDuration: $theme.animation.timing500,
          }),
        },
        Content: {
          style: {
            padding: 0,
            // paddingBottom appears provided by the StyledContent component controlled by
            // baseui overrides padding: 0
            paddingBottom: 0,
          },
        },
        Header: {
          style: {
            padding: 0,
          },
        },
        ToggleIcon: {
          style: {
            alignSelf: 'flex-start',
          },
        },
      }}
    />
  );
}
