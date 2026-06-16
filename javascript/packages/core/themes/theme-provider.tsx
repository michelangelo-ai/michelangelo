import { useContext } from 'react';
import { BaseProvider, createTheme } from 'baseui';
import { LayersContext } from 'baseui/layer';
import { ThemeProvider as BaseUIThemeProvider } from 'baseui/styles';

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
  const { host } = useContext(LayersContext);
  const hasParentProvider = host !== undefined;

  // TODO: rename Icons to be PascalCase #364
  const iconEntries = icons
    ? Object.fromEntries(
        Object.entries(icons).map(([key, value]) => [capitalizeFirstLetter(key), value])
      )
    : {};

  const resolvedTheme = theme ?? createTheme({ ...GRID_OVERRIDES, icons: iconEntries });

  if (hasParentProvider) {
    return <BaseUIThemeProvider theme={resolvedTheme}>{children}</BaseUIThemeProvider>;
  }

  return <BaseProvider theme={resolvedTheme}>{children}</BaseProvider>;
}
