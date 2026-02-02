import React from 'react';
import { getOverrides } from 'baseui';
import { Skeleton } from 'baseui/skeleton';

import { getObjectValue } from '#core/utils/object-utils';
import { RowItem as BaseRowItem } from './components/row-item';
import { StyledRowContainer, StyledRowItemContainer } from './styled-components';

import type { RowProps } from '#core/components/row/types';

/**
 * Displays data as a horizontal row of label-value pairs with optional loading state.
 *
 * Row provides a flexible layout for displaying record data in a structured format,
 * with automatic handling of empty values, loading skeletons, and value extraction
 * from data records using accessors.
 *
 * Features:
 * - Horizontal layout of label-value pairs
 * - Automatic value extraction via accessors (string paths or functions)
 * - Optional hiding of empty values via hideEmpty flag
 * - Loading state with skeleton UI
 * - Customizable through BaseUI overrides
 * - Theme integration
 *
 * @param props.items - Array of row items to display, each with id, label, and optional accessor
 * @param props.record - Data record to extract values from
 * @param props.loading - When true, displays skeleton loading placeholders instead of values
 * @param props.overrides - BaseUI overrides for RowContainer, RowItemContainer, and RowItem components
 *
 * @example
 * ```tsx
 * // Basic row with data
 * <Row
 *   record={{ name: 'training-pipeline', version: 'v1.2', status: 'running' }}
 *   items={[
 *     { id: 'name', label: 'Name', accessor: 'name' },
 *     { id: 'version', label: 'Version', accessor: 'version' },
 *     { id: 'status', label: 'Status', accessor: 'status' }
 *   ]}
 * />
 *
 * // With loading state
 * <Row
 *   loading={true}
 *   items={[
 *     { id: 'name', label: 'Name' },
 *     { id: 'status', label: 'Status' }
 *   ]}
 * />
 *
 * // Hide empty values
 * <Row
 *   record={{ name: 'pipeline', description: null }}
 *   items={[
 *     { id: 'name', label: 'Name', accessor: 'name' },
 *     { id: 'description', label: 'Description', accessor: 'description', hideEmpty: true }
 *   ]}
 * />
 * // Only "Name" will be displayed
 * ```
 */
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
