import { styled } from 'baseui';

import { STATE_TO_STYLE_MAP } from '#core/components/views/execution/constants';
import { ELLIPSIS_STYLES } from '#core/styles/constants';

import type { TaskState } from '#core/components/views/execution/types';

/**
 * Interactive card container for individual task display with state-based styling.
 * Applies colors, borders, and backgrounds based on task execution state.
 *
 * @param $state - Task execution state that determines visual styling
 */
export const TaskStepCardContainer = styled<'div', { role?: string; $state: TaskState }>(
  'div',
  ({ $theme: { colors, sizing }, $state, role }) => {
    const {
      colorName = 'contentPrimary',
      backgroundColorName = 'backgroundPrimary',
      borderColorName,
    } = STATE_TO_STYLE_MAP[$state];

    return {
      flex: '1 1 0',
      position: 'relative',
      display: 'flex',
      alignItems: 'center',
      gap: sizing.scale300,
      height: '44px',
      padding: '0 16px',
      boxSizing: 'border-box',
      border: `2px solid ${colors[borderColorName]}`,
      borderRadius: '12px',
      backgroundColor: colors[backgroundColorName],
      color: colors[colorName],
      cursor: role === 'button' ? 'pointer' : 'default',
    };
  }
);

export const TaskStepName = styled('div', ({ $theme: { typography } }) => ({
  ...typography.LabelSmall,
  ...ELLIPSIS_STYLES,
}));
