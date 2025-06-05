import { TruncatedText } from '#core/components/truncated-text/truncated-text';

import type { CellRenderer } from '#core/components/cell/types';

export const TextCell: CellRenderer<string> = (props) => {
  return <TruncatedText>{props.value}</TruncatedText>;
};
