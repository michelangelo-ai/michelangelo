import { styled } from 'baseui';

export const Container = styled('div', ({ $theme }) => ({
  display: 'flex',
  flexDirection: 'column',
  gap: $theme.sizing.scale300,
}));

export const ActionsContainer = styled('div', ({ $theme }) => ({
  alignItems: 'center',
  display: 'flex',
  gap: $theme.sizing.scale300,
}));

export const TrailingContentContainer = styled('div', ({ $theme }) => ({
  display: 'flex',
  alignItems: 'center',
  gap: $theme.sizing.scale300,
  marginLeft: 'auto',
}));
