import { memo } from 'react';
import { useStyletron } from 'baseui';
import { Icon as BaseIcon } from 'baseui/icon';

import { useIconProvider } from '#core/providers/icon-provider/use-icon-provider';
import { capitalizeFirstLetter } from '#core/utils/string-utils';
import { IconKind } from './types';

import type { Props } from './types';

export const Icon = memo<Props>(function Icon(props: Props) {
  const [, theme] = useStyletron();
  const { color, name, icon, kind = IconKind.PRIMARY } = props;
  const { icons } = useIconProvider();

  const IconComponent = icon ?? (name ? icons[name] : null);

  if (!IconComponent) return null;

  return (
    <BaseIcon
      {...props}
      color={
        color ?? theme.colors[`content${capitalizeFirstLetter(kind)}` as keyof typeof theme.colors]
      }
    >
      <IconComponent style={{ minWidth: 'fit-content' }} />
    </BaseIcon>
  );
});
