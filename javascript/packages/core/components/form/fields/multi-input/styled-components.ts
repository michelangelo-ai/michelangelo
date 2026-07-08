import { styled } from 'baseui';

export const TagList = styled('div', ({ $theme }) => ({
  display: 'flex',
  flexWrap: 'wrap',
  gap: $theme.sizing.scale300,
  marginTop: $theme.sizing.scale300,
}));
