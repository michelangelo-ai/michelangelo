import { Box } from '#core/components/box/box';
import { Row } from '#core/components/row/row';
import { mapLinkConfigToColumnConfig } from './map-link-config-to-column-config';

import type { LinksBoxProps } from './types';

/**
 * Renders a titled box with a row of related links. Links missing a name or
 * url are skipped.
 */
export function LinksBox(props: LinksBoxProps) {
  const { links } = props;
  return (
    <Box title="Useful links">
      <Row record={{}} items={mapLinkConfigToColumnConfig(links)} />
    </Box>
  );
}
