import { styled } from 'baseui';

export const SignpostContainer = styled('div', ({ $theme }) => ({
  textAlign: 'center',
  margin: '90px auto',
  maxWidth: '450px',

  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  gap: $theme.sizing.scale600,
}));
