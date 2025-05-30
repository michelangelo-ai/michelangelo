import React from 'react';

import { DateTime } from '#core/components/date-time/date-time';
import { dateCellToString } from './date-cell-to-string';

import type { CellRendererProps } from '#core/components/cell/types';

export const DateCell = (props: CellRendererProps<string>): React.ReactElement => {
  return <DateTime timestamp={props.value} />;
};

DateCell.toString = dateCellToString;
