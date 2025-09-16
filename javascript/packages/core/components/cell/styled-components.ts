import { styled } from 'baseui';

import type { Theme } from 'baseui';

export const CellContainer = styled('div', ({ $theme }: { $theme: Theme }) => ({
  display: 'flex',
  gap: $theme.sizing.scale100,
  alignItems: 'center',
}));
