import { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from './constants';

import type { Theme } from 'baseui';
import type { ColorOverrides, TagBehavior, TagColor, TagHierarchy, TagSize } from './types';

const COLOR_OVERRIDES: ColorOverrides = {
  [TAG_HIERARCHY.primary]: {
    [TAG_BEHAVIOR.display]: {
      [TAG_COLOR.purple]: (theme: Theme) => ({
        color: theme.colors.white,
        backgroundColor: theme.colors.purple600,
      }),
      [TAG_COLOR.magenta]: (theme: Theme) => ({
        color: theme.colors.white,
        backgroundColor: theme.colors.magenta600,
      }),
    },
    [TAG_BEHAVIOR.selection]: {
      [TAG_COLOR.gray]: (theme: Theme) => ({
        borderColor: theme.colors.gray700,
      }),
      [TAG_COLOR.red]: (theme: Theme) => ({
        borderColor: theme.colors.red700,
      }),
      [TAG_COLOR.orange]: (theme: Theme) => ({
        borderColor: theme.colors.orange700,
      }),
      [TAG_COLOR.yellow]: (theme: Theme) => ({
        borderColor: theme.colors.yellow700,
      }),
      [TAG_COLOR.green]: (theme: Theme) => ({
        borderColor: theme.colors.green700,
      }),
      [TAG_COLOR.blue]: (theme: Theme) => ({
        borderColor: theme.colors.blue700,
      }),
      [TAG_COLOR.purple]: (theme: Theme) => ({
        color: theme.colors.white,
        backgroundColor: theme.colors.purple600,
        borderColor: theme.colors.purple700,
      }),
      [TAG_COLOR.magenta]: (theme: Theme) => ({
        color: theme.colors.white,
        backgroundColor: theme.colors.magenta600,
        borderColor: theme.colors.magenta700,
      }),
      [TAG_COLOR.teal]: (theme: Theme) => ({
        borderColor: theme.colors.teal700,
      }),
      [TAG_COLOR.lime]: (theme: Theme) => ({
        borderColor: theme.colors.lime700,
      }),
    },
  },
  [TAG_HIERARCHY.secondary]: {
    [TAG_BEHAVIOR.display]: {
      [TAG_COLOR.magenta]: (theme: Theme) => ({
        color: theme.colors.magenta700,
        backgroundColor: theme.colors.magenta50,
      }),
      [TAG_COLOR.purple]: (theme: Theme) => ({
        color: theme.colors.purple700,
        backgroundColor: theme.colors.purple50,
      }),
    },
    [TAG_BEHAVIOR.selection]: {
      [TAG_COLOR.gray]: (theme: Theme) => ({
        borderColor: theme.colors.gray100,
      }),
      [TAG_COLOR.red]: (theme: Theme) => ({
        borderColor: theme.colors.red100,
      }),
      [TAG_COLOR.orange]: (theme: Theme) => ({
        borderColor: theme.colors.orange100,
      }),
      [TAG_COLOR.yellow]: (theme: Theme) => ({
        borderColor: theme.colors.yellow100,
      }),
      [TAG_COLOR.green]: (theme: Theme) => ({
        borderColor: theme.colors.green100,
      }),
      [TAG_COLOR.blue]: (theme: Theme) => ({
        borderColor: theme.colors.blue100,
      }),
      [TAG_COLOR.purple]: (theme: Theme) => ({
        color: theme.colors.purple700,
        backgroundColor: theme.colors.purple50,
        borderColor: theme.colors.purple100,
      }),
      [TAG_COLOR.magenta]: (theme: Theme) => ({
        color: theme.colors.magenta700,
        backgroundColor: theme.colors.magenta50,
        borderColor: theme.colors.magenta100,
      }),
      [TAG_COLOR.teal]: (theme: Theme) => ({
        borderColor: theme.colors.teal100,
      }),
      [TAG_COLOR.lime]: (theme: Theme) => ({
        borderColor: theme.colors.lime100,
      }),
    },
  },
} as const;

export const getTagOverrides = (
  theme: Theme,
  {
    size,
    behavior,
    color,
    hierarchy,
  }: {
    size: TagSize;
    behavior: TagBehavior;
    color: TagColor;
    hierarchy: TagHierarchy;
  }
) => {
  const styles = COLOR_OVERRIDES[hierarchy]?.[behavior]?.[color]?.(theme);

  return {
    Root: {
      style: {
        ...(size === TAG_SIZE.xSmall
          ? {
              ...theme.typography.LabelXSmall,
              height: '20px',
              paddingLeft: theme.sizing.scale100,
              paddingRight: theme.sizing.scale100,
            }
          : {}),
        ...(styles?.borderColor
          ? {
              border: `1px solid ${styles.borderColor}`,
            }
          : {}),
        ...(styles?.color ? { color: styles.color } : {}),
        ...(styles?.backgroundColor ? { backgroundColor: styles.backgroundColor } : {}),
        margin: 0,
        verticalAlign: 'bottom',
        justifyContent: 'center',
      },
    },

    StartEnhancerContainer: {
      style: {
        ...(size === TAG_SIZE.xSmall
          ? {
              paddingRight: theme.sizing.scale100,
            }
          : {}),
      },
    },

    Text: {
      style: {
        display: 'flex',
        alignItems: 'center',
      },
    },
  };
};
