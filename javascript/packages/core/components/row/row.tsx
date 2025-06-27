import React from 'react';
import { getOverrides } from 'baseui';
import { Skeleton } from 'baseui/skeleton';

import { getObjectValue } from '#core/utils/object-utils';
import { RowItem as BaseRowItem } from './components/row-item';
import { StyledRowContainer, StyledRowItemContainer } from './styled-components';

import type { RowProps } from '#core/components/row/types';

export function Row(props: RowProps) {
  const { items, loading = false, record = {}, overrides = {} } = props;

  const [RowContainer, rowContainerProps] = getOverrides(
    overrides.RowContainer,
    StyledRowContainer
  );

  const [RowItemContainer, rowItemContainerProps] = getOverrides(
    overrides.RowItemContainer,
    StyledRowItemContainer
  );

  const [RowItem, rowItemProps] = getOverrides(overrides.RowItem, BaseRowItem);

  return (
    <RowContainer {...rowContainerProps}>
      {items
        .filter((item) => {
          if (loading) {
            return true;
          }
          const value = getObjectValue(record, item.accessor ?? item.id);
          if (!item.hideEmpty) {
            return true;
          }
          return value !== undefined && value !== null;
        })
        .map((item, i) => (
          <React.Fragment key={i}>
            {loading && (
              <Skeleton
                animation
                height="48px"
                width="120px"
                overrides={{
                  Root: {
                    props: {
                      'data-testid': 'loading',
                    },
                  },
                }}
              />
            )}
            {!loading && (
              <RowItemContainer $index={i} {...rowItemContainerProps}>
                {<RowItem item={item} record={record} {...rowItemProps} />}
              </RowItemContainer>
            )}
          </React.Fragment>
        ))}
    </RowContainer>
  );
}
