import { styled } from 'baseui';

export const ErrorViewContainer = styled('div', ({ $theme: { sizing, typography } }) => ({
  ...typography.HeadingXSmall,
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  gap: sizing.scale700,
  textAlign: 'center',
}));

export const TitleContainer = styled('div', ({ $theme: { colors, typography } }) => ({
  ...typography.HeadingXSmall,
  color: colors.primary,
}));

export const DescriptionContainer = styled('div', ({ $theme: { colors, typography } }) => ({
  ...typography.ParagraphMedium,
  color: colors.primary,
}));
