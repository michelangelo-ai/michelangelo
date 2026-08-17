import { styled, useStyletron } from 'baseui';
import { StyledToggleIcon } from 'baseui/accordion';

import { InterpolatableActionsPopover } from '#core/components/actions/interpolatable-actions-popover';
import { CollapsibleBox } from '#core/components/box/collapsible-box';
import { STATE_TO_STYLE_MAP } from '#core/components/views/execution/constants';

import type { Theme } from 'baseui';
import type { SharedStylePropsArg } from 'baseui/accordion';
import type { ReactNode } from 'react';
import type { ActionConfigSchema, Data } from '#core/components/actions/types';
import type { CollapsibleBoxProps } from '#core/components/box/types';
import type { TaskState } from '#core/components/views/execution/types';

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
  width: '100%',
}));

type TaskPanelProps = CollapsibleBoxProps & {
  id?: string;
  state?: TaskState;
  actions?: ActionConfigSchema<Data>[];
  actionRecord?: Data;
};

export function TaskPanel(props: TaskPanelProps) {
  const {
    id,
    defaultExpanded,
    state,
    actions,
    actionRecord,
    overrides: userOverrides,
    ...collapsibleBoxProps
  } = props;
  const [css, theme] = useStyletron();

  const taskPanelOverrides = {
    Container: {
      props: {
        id,
        onClick: (e: MouseEvent) => e.stopPropagation(),
      },
      ...(state && {
        style: ({ $theme }: { $theme: Theme }) => ({
          borderColor: $theme.colors[STATE_TO_STYLE_MAP[state].borderColorName],
        }),
      }),
    },
    Content: {
      style: ({ $theme }: { $theme: Theme }) => ({
        // When combined with CollapsibleBox gap between content and header, results in designed
        // spacing of 24px
        paddingTop: $theme.sizing.scale300,
      }),
    },
    // Rendered as a sibling of the real toggle icon (rather than inline in the title content)
    // so the actions button sits flush against the chevron regardless of how wide the title is.
    ...(!!actions?.length && {
      ToggleIcon: {
        component: ({
          children,
          ...toggleIconProps
        }: { children: ReactNode } & SharedStylePropsArg) => (
          <div
            className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale300 })}
          >
            <InterpolatableActionsPopover actions={actions} record={actionRecord ?? {}} />
            <StyledToggleIcon {...toggleIconProps}>{children}</StyledToggleIcon>
          </div>
        ),
      },
    }),
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
