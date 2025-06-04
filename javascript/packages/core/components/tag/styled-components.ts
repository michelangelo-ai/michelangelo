import { BEHAVIOR, COLOR, HIERARCHY, SIZE } from './constants';

import type { Theme } from 'baseui';
import type { ColorOverrides, TagBehavior, TagColor, TagHierarchy, TagSize } from './types';

const COLOR_OVERRIDES: ColorOverrides = {
  [HIERARCHY.primary]: {
    [BEHAVIOR.display]: {
      [COLOR.purple]: (theme: Theme) => ({
        color: theme.colors.white,
        backgroundColor: theme.colors.purple600,
      }),
      [COLOR.magenta]: (theme: Theme) => ({
        color: theme.colors.white,
        backgroundColor: theme.colors.magenta600,
      }),
    },
    [BEHAVIOR.selection]: {
      [COLOR.gray]: (theme: Theme) => ({
        borderColor: theme.colors.gray700,
      }),
      [COLOR.red]: (theme: Theme) => ({
        borderColor: theme.colors.red700,
      }),
      [COLOR.orange]: (theme: Theme) => ({
        borderColor: theme.colors.orange700,
      }),
      [COLOR.yellow]: (theme: Theme) => ({
        borderColor: theme.colors.yellow700,
      }),
      [COLOR.green]: (theme: Theme) => ({
        borderColor: theme.colors.green700,
      }),
      [COLOR.blue]: (theme: Theme) => ({
        borderColor: theme.colors.blue700,
      }),
      [COLOR.purple]: (theme: Theme) => ({
        color: theme.colors.white,
        backgroundColor: theme.colors.purple600,
        borderColor: theme.colors.purple700,
      }),
      [COLOR.magenta]: (theme: Theme) => ({
        color: theme.colors.white,
        backgroundColor: theme.colors.magenta600,
        borderColor: theme.colors.magenta700,
      }),
      [COLOR.teal]: (theme: Theme) => ({
        borderColor: theme.colors.teal700,
      }),
      [COLOR.lime]: (theme: Theme) => ({
        borderColor: theme.colors.lime700,
      }),
    },
  },
  [HIERARCHY.secondary]: {
    [BEHAVIOR.display]: {
      [COLOR.magenta]: (theme: Theme) => ({
        color: theme.colors.magenta700,
        backgroundColor: theme.colors.magenta50,
      }),
      [COLOR.purple]: (theme: Theme) => ({
        color: theme.colors.purple700,
        backgroundColor: theme.colors.purple50,
      }),
    },
    [BEHAVIOR.selection]: {
      [COLOR.gray]: (theme: Theme) => ({
        borderColor: theme.colors.gray100,
      }),
      [COLOR.red]: (theme: Theme) => ({
        borderColor: theme.colors.red100,
      }),
      [COLOR.orange]: (theme: Theme) => ({
        borderColor: theme.colors.orange100,
      }),
      [COLOR.yellow]: (theme: Theme) => ({
        borderColor: theme.colors.yellow100,
      }),
      [COLOR.green]: (theme: Theme) => ({
        borderColor: theme.colors.green100,
      }),
      [COLOR.blue]: (theme: Theme) => ({
        borderColor: theme.colors.blue100,
      }),
      [COLOR.purple]: (theme: Theme) => ({
        color: theme.colors.purple700,
        backgroundColor: theme.colors.purple50,
        borderColor: theme.colors.purple100,
      }),
      [COLOR.magenta]: (theme: Theme) => ({
        color: theme.colors.magenta700,
        backgroundColor: theme.colors.magenta50,
        borderColor: theme.colors.magenta100,
      }),
      [COLOR.teal]: (theme: Theme) => ({
        borderColor: theme.colors.teal100,
      }),
      [COLOR.lime]: (theme: Theme) => ({
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
        ...(size === SIZE.xSmall
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
        ...(size === SIZE.xSmall
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
