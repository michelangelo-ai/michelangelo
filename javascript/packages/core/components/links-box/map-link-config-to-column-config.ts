import type { LinkCellConfig } from '#core/components/cell/renderers/link/types';
import type { LinkConfig } from './types';

export function mapLinkConfigToColumnConfig(links: LinkConfig[]): LinkCellConfig[] {
  return links
    .filter((link): link is Required<LinkConfig> => !!link.name && !!link.url)
    .map((link) => ({
      id: link.name,
      accessor: () => link.name,
      url: link.url,
      tooltip: link.tooltip,
    }));
}
