import { styled } from 'baseui';

import type { StyleObject } from 'styletron-react';

export const DescriptionText = styled<'div', { $styleOverrides?: StyleObject }>(
  'div',
  ({ $theme, $styleOverrides }) => ({
    ...$theme.typography.ParagraphSmall,
    color: $theme.colors.contentSecondary,
    display: 'flex',
    alignItems: 'center',
    ...$styleOverrides,
  })
);
