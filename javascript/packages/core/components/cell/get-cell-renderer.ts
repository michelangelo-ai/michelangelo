import { CELL_RENDERERS } from './constants';
import { CellRenderer } from './types';

export function getCellRenderer(type: string): CellRenderer<unknown> | undefined {
  return CELL_RENDERERS[type];
}
