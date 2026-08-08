import { styled } from 'baseui';

import { MessageLevel } from './types';

import type { Theme } from 'baseui';

export const StyledRoot = styled<'div', { $level: MessageLevel }>('div', ({ $level, $theme }) => ({
  backgroundColor: getBackgroundColorStyle($level, $theme),
  borderRadius: $theme.borders.inputBorderRadius,
  display: 'flex',
  flexDirection: 'column',
  gap: $theme.sizing.scale100,
  minHeight: '91px',
  overflow: 'auto',
  padding: $theme.sizing.scale600,
  resize: 'vertical',
}));

function getBackgroundColorStyle(level: MessageLevel, theme: Theme) {
  switch (level) {
    case MessageLevel.ERROR:
      return theme.colors.backgroundLightNegative;
    case MessageLevel.WARNING:
      return theme.colors.backgroundLightWarning;
    default:
      return theme.colors.backgroundSecondary;
  }
}

export const StyledContent = styled('div', ({ $theme }) => ({
  ...$theme.typography.ParagraphSmall,
}));
