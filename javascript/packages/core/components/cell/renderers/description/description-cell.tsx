import { useStyletron } from 'baseui';

import { DescriptionText } from '#core/components/description-text';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';
import { DescriptionHierarchy } from './constants';

import type { CellRendererProps } from '#core/components/cell/types';
import type { DescriptionCellConfig } from './types';

export const DescriptionCell = ({
  column,
  value,
}: CellRendererProps<string, DescriptionCellConfig>) => {
  const [, theme] = useStyletron();
  return (
    <DescriptionText
      {...(column.hierarchy === DescriptionHierarchy.PRIMARY
        ? { $styleOverrides: { color: theme.colors.contentPrimary } }
        : {})}
    >
      <TruncatedText>{value}</TruncatedText>
    </DescriptionText>
  );
};
