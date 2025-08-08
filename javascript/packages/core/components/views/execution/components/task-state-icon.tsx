import { useStyletron } from 'baseui';

import { Icon } from '#core/components/icon/icon';
import { STATE_TO_ICON_MAP } from '#core/components/views/execution/constants';

import type { TaskState } from '#core/components/views/execution/types';

export function TaskStateIcon(props: { state: TaskState }) {
  const { state } = props;
  const [, { colors, sizing }] = useStyletron();
  const iconProps = STATE_TO_ICON_MAP[state];

  return <Icon name={iconProps.name} color={colors[iconProps.colorName]} size={sizing.scale750} />;
}
