import type { Theme } from 'baseui';

export const getSelectionColumnCellStyles = (theme: Theme) => {
  return {
    width: theme.sizing.scale900,
    minWidth: theme.sizing.scale900,
    maxWidth: theme.sizing.scale900,
  } as const;
};
