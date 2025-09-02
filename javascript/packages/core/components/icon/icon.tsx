import { memo } from 'react';
import { useStyletron } from 'baseui';

import { useIconProvider } from '#core/providers/icon-provider/use-icon-provider';
import { capitalizeFirstLetter } from '#core/utils/string-utils';
import { IconKind } from './types';

import type { Props } from './types';

export const Icon = memo<Props>(function Icon(props: Props) {
  const [, theme] = useStyletron();
  const { color, name, icon, kind = IconKind.PRIMARY, size = theme.sizing.scale550 } = props;
  const { icons } = useIconProvider();

  const IconComponent = icon ?? (name ? icons[name] : null);

  if (!IconComponent) return null;

  return (
    <IconComponent
      {...props}
      size={size}
      color={color ?? theme.colors[`content${capitalizeFirstLetter(kind)}`]}
      style={{ minWidth: 'fit-content' }}
    />
  );
});
