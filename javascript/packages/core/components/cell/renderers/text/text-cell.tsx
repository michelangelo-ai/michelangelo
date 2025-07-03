import { useCellToString } from '#core/components/cell/use-cell-to-string';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';

import type { CellRenderer } from '#core/components/cell/types';

export const TextCell: CellRenderer<string> = (props) => {
  const cellToString = useCellToString();
  return <TruncatedText>{cellToString(props) ?? '\u2014'}</TruncatedText>;
};
