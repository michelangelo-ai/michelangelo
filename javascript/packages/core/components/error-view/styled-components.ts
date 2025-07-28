import { styled } from 'baseui';

export const ErrorViewContainer = styled('div', ({ $theme }) => ({
  textAlign: 'center',
  margin: '90px auto',
  maxWidth: '450px',

  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  gap: $theme.sizing.scale600,
}));
