import React from 'react';
import { Skeleton } from 'baseui/skeleton';

import { Body, CellContainer, Row } from '../styled-components';

export function TableLoadingState() {
  return (
    <Body data-testid="table-loading-state">
      {[1, 2, 3].map((row) => (
        <Row key={row}>
          <CellContainer colSpan={100}>
            <Skeleton animation width="100%" height="22px" />
          </CellContainer>
        </Row>
      ))}
    </Body>
  );
}
