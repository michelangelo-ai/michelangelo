import { cellToString } from '#core/components/cell/cell-to-string';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';

import type { CellRendererProps } from '#core/components/cell/types';

export function linkCellToString(props: CellRendererProps<string>) {
  return <TruncatedText>{cellToString(props) ?? props.value}</TruncatedText>;
}
