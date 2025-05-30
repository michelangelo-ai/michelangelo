import { useStyletron } from 'baseui';

import type { StyleObject } from 'styletron-react';
import type { SharedCell } from './types';

export function useCellStyles({
  record,
  style,
}: {
  record: unknown;
  style: SharedCell['style'] | undefined;
}): StyleObject {
  const [, theme] = useStyletron();

  if (!style) return {};

  if (typeof style !== 'function') {
    return style;
  }

  return style({ record, theme });
}
