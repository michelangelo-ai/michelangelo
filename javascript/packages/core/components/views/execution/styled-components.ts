import { styled } from 'baseui';

export const TaskSeparator = styled('hr', ({ $theme }) => ({
  border: 'none',
  borderBottom: `2px dashed ${$theme.colors.contentInverseTertiary}`,
  margin: `${$theme.sizing.scale200} 0`,
}));
