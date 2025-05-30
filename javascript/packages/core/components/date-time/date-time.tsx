import { useUserProvider } from '#core/providers/user-provider/use-user-provider';
import { timestampToString } from '#core/utils/time-utils';

import type { Props } from './types';

export function DateTime(props: Props) {
  const { timestamp } = props;

  const { timeZone } = useUserProvider();

  const dateString = timestamp ? timestampToString(timestamp, timeZone) : null;

  return <>{dateString}</>;
}
