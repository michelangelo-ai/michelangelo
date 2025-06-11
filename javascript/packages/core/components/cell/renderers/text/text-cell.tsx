import { TruncatedText } from '#core/components/truncated-text/truncated-text';
import { cellToString } from '#core/components/cell/cell-to-string';

import type { CellRenderer } from '#core/components/cell/types';

export const TextCell: CellRenderer<string> = (props) => {
  return <TruncatedText>{cellToString(props) ?? '\u2014'}</TruncatedText>;
};
