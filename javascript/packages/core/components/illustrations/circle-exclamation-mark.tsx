import { Alert } from 'baseui/icon';

import type { IllustrationProps } from './types';

export const CircleExclamationMark = ({ width = '64' }: IllustrationProps) => (
  <Alert size={width} color="error" />
);
