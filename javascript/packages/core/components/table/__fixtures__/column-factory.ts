import { merge } from 'lodash';

import type { DeepPartial } from '#core/types/utility-types';
import type { ColumnConfig } from '../types/column-types';

/**
 * Factory for creating ColumnConfig test fixtures.
 * Provides minimal required properties for testing with sensible defaults.
 *
 * @param base - Partial object shared across all test fixtures for a test suite
 * @returns Function that generates a complete column configuration using overrides.
 *
 * @example
 * ```typescript
 * // Basic usage
 * const buildColumn = buildColumnFactory();
 * const column = buildColumn({ id: 'name', label: 'Name' });
 *
 * // With base configuration for test suite
 * const buildTooltipColumn = buildColumnFactory({
 *   tooltip: { action: 'filter' }
 * });
 * const nameColumn = buildTooltipColumn({
 *   id: 'metadata.name',
 *   tooltip: { content: 'Click to filter by name' }
 * });
 * ```
 */
export const buildColumnFactory = (base: DeepPartial<ColumnConfig> = {}) => {
  return (overrides: DeepPartial<ColumnConfig> = {}): ColumnConfig => {
    const required: ColumnConfig = {
      id: 'test-column',
      label: 'Test Column',
    };

    return merge({}, required, base, overrides);
  };
};
