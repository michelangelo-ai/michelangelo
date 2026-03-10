import type { Theme } from 'baseui';

export const TILE_GROUP_OVERRIDES = {
  RadioMarkOuter: {
    style: ({ $disabled, $theme }: { $disabled: boolean; $theme: Theme }) => {
      return $disabled ? { backgroundColor: $theme.colors.tickFillSelected } : {};
    },
  },
};

export const TILE_OVERRIDES = {
  Root: {
    style: ({ $selected, $theme }: { $selected: boolean; $theme: Theme }) => ({
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'flex-start',
      justifyContent: 'flex-start',
      flex: '1 1 0%',
      ':disabled': {
        opacity: 0.5,
        ...($selected
          ? { boxShadow: `inset 0px 0px 0px 3px ${$theme.colors.borderSelected}` }
          : {}),
      },
    }),
  },
  HeaderContainer: {
    style: {
      width: '100%',
      marginBottom: 0,
    },
  },
};
