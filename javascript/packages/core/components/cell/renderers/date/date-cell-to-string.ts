import { timestampToString } from '#core/utils/time-utils';

import type { CellToStringParams } from '#core/components/cell/types';

export const dateCellToString = ({ value }: CellToStringParams<string>) =>
  timestampToString(value) ?? '';
