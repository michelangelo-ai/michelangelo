import { styled } from 'baseui';

import type { ColorTokens } from 'baseui/styles';

export const TaskFlowContainer = styled('div', ({ $theme }) => ({
  display: 'flex',
  flex: 'auto',
  gap: $theme.sizing.scale300,
  flexWrap: 'wrap',
  alignItems: 'center',
}));

/**
 * CSS triangle arrow indicator for showing flow direction between tasks.
 *
 * @param $color - Theme color token for the arrow fill
 * @param $direction - Arrow orientation: 'right' for horizontal flow, 'up' for vertical flow
 */
export const TaskIndicator = styled<
  'div',
  { $color: keyof ColorTokens; $direction: 'right' | 'up' }
>('div', ({ $theme, $color, $direction }) => {
  const isUp = $direction === 'up';

  return {
    width: 0,
    height: 0,

    borderLeft: isUp ? '10px solid transparent' : `8px solid ${$theme.colors[$color]}`,
    borderRight: isUp ? '10px solid transparent' : '0px solid transparent',
    borderTop: isUp ? '8px solid transparent' : '10px solid transparent',
    borderBottom: isUp ? `8px solid ${$theme.colors[$color]}` : '10px solid transparent',
  };
});
