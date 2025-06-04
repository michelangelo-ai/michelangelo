import { sentenceCaseEnumValue } from '#core/utils/string-utils';

import type { CellToStringParams } from '#core/components/cell/types';
import type { StateCellConfig } from './types';

export const stateToString = ({
  column,
  value,
}: CellToStringParams<string, StateCellConfig>): string => {
  if (!value) return '';
  if (column.stateTextMap?.[value]) return column.stateTextMap[value];
  if (value.endsWith('_INVALID')) return 'Queued';
  return sentenceCaseEnumValue(value, /(\w+_)STATE_/);
};
