import { useStyletron } from 'baseui';

import { Icon } from '#core/components/icon/icon';

export function TableExpandIcon({ expanded }: { expanded: boolean }) {
  const [, theme] = useStyletron();
  const iconName = expanded ? 'chevronDown' : 'chevronRight';
  const title = expanded ? 'Collapse' : 'Expand';

  return <Icon name={iconName} title={title} size={theme.sizing.scale600} />;
}
