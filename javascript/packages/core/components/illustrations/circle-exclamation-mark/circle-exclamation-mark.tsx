import { Alert } from 'baseui/icon';

import { CircleExclamationMarkKind } from './types';

import type { CircleExclamationMarkProps } from './types';

export const CircleExclamationMark = ({
  width = '64',
  kind = CircleExclamationMarkKind.ERROR,
}: CircleExclamationMarkProps) => (
  <Alert
    size={width}
    color={kind === CircleExclamationMarkKind.ERROR ? 'contentNegative' : 'contentPrimary'}
  />
);
