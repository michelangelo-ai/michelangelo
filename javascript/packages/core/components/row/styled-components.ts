import { styled } from 'baseui';

export const StyledRowContainer = styled('div', ({ $theme }) => ({
  display: 'flex',
  gap: $theme.sizing.scale950,
  flexWrap: 'wrap',
  width: '100%',
}));

export const StyledRowItemContainer = styled<'div', { $index: number }>('div', ({ $index }) => ({
  animation: `0.3s ease ${$index * 0.06}s 1 both`,
  animationName: {
    from: { opacity: '0' },
    to: { opacity: '1' },
  },
}));
