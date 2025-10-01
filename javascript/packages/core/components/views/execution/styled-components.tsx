import { styled, Theme } from 'baseui';

import { CollapsibleBox } from '#core/components/box/collapsible-box';

import type { CollapsibleBoxProps } from '#core/components/box/types';

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

export function TaskPanel(props: CollapsibleBoxProps & { id?: string }) {
  const { id, defaultExpanded, overrides: userOverrides, ...collapsibleBoxProps } = props;

  const taskPanelOverrides = {
    Container: {
      props: {
        id,
        onClick: (e: MouseEvent) => e.stopPropagation(),
      },
    },
    Content: {
      style: ({ $theme }: { $theme: Theme }) => ({
        // When combined with CollapsibleBox gap between content and header, results in designed
        // spacing of 24px
        paddingTop: $theme.sizing.scale300,
      }),
    },
    ...userOverrides,
  };

  return (
    <CollapsibleBox
      {...collapsibleBoxProps}
      defaultExpanded={defaultExpanded ?? false}
      overrides={taskPanelOverrides}
    />
  );
}
