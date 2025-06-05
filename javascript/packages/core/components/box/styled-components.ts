import { styled } from 'baseui';

export const StyledBoxContainer = styled('div', ({ $theme }) => ({
  ...$theme.borders.border200,
  borderRadius: $theme.borders.radius400,
  display: 'flex',
  flexDirection: 'column',
  gap: $theme.sizing.scale600,
  padding: $theme.sizing.scale800,
}));

export const StyledBoxHeader = styled('div', {
  display: 'flex',
  flexDirection: 'column',
  width: '100%',
});

export const StyledBoxTitle = styled('div', ({ $theme }) => ({
  ...$theme.typography.LabelLarge,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'flex-start',
  gap: $theme.sizing.scale100,
}));
