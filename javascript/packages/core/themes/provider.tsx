import { BaseProvider, createTheme } from 'baseui';

import { capitalizeFirstLetter } from '#core/utils/string-utils';
import { GRID_OVERRIDES } from './shared';

import type { Theme } from 'baseui';
import type { IconMap } from '#core/providers/icon-provider/types';

export function ThemeProvider({
  children,
  icons,
  theme,
}: {
  children: React.ReactNode;
  icons?: IconMap;
  theme?: Theme;
}) {
  // TODO: rename Icons to be PascalCase #364
  const iconEntries = icons
    ? Object.fromEntries(
        Object.entries(icons).map(([key, value]) => [capitalizeFirstLetter(key), value])
      )
    : {};

  return (
    <BaseProvider theme={theme ?? createTheme({ ...GRID_OVERRIDES, icons: iconEntries })}>
      {children}
    </BaseProvider>
  );
}
