import { Icon } from '#core/components/icon/icon';

export function TableExpandIcon({ expanded }: { expanded: boolean }) {
  const iconName = expanded ? 'chevronDown' : 'chevronRight';
  const title = expanded ? 'Collapse' : 'Expand';

  return <Icon name={iconName} title={title} size={16} />;
}
