import type { CellTooltip } from '#core/components/cell/types';

export type LinksBoxProps = {
  links: LinkConfig[];
};

export type LinkConfig = { url?: string; name?: string; tooltip?: CellTooltip };
