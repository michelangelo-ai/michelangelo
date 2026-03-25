import { CellProvider } from '#core/providers/cell-provider/cell-provider';

import type { CellRenderer } from '#core/components/cell/types';
import type { WrapperComponentProps } from './types';

export function getCellProviderWrapper({
  renderers = {},
}: { renderers?: Record<string, CellRenderer<unknown>> } = {}) {
  return function CellProviderWrapper({ children }: WrapperComponentProps) {
    return <CellProvider renderers={renderers}>{children}</CellProvider>;
  };
}
