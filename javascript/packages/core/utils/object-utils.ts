import { get } from 'lodash';

import type { Accessor } from '#core/types/common/studio-types';

export function getObjectValue<K>(
  obj: object,
  accessor: Accessor<K>,
  defaultValue?: K
): K | undefined {
  if (typeof accessor === 'function') {
    return accessor(obj) || defaultValue;
  }

  if (typeof accessor === 'string') {
    return get(obj, accessor, defaultValue);
  }

  return undefined;
}
