import { styled } from 'baseui';

export const DetailHeaderContainer = styled('div', ({ $theme }) => ({
  ...$theme.borders.border200,
  backgroundColor: $theme.colors.backgroundSecondary,
  borderRadius: $theme.borders.radius400,
  display: 'flex',
  flexDirection: 'column',
  gap: $theme.sizing.scale600,
  padding: $theme.sizing.scale800,
}));
