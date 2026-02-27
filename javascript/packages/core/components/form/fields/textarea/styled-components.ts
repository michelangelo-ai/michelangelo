import { styled } from 'baseui';

export const StyledMaxLengthContainer = styled('span', ({ $theme }) => ({
  ...$theme.typography.LabelSmall,
  color: $theme.colors.contentTertiary,
}));
