import { merge } from 'lodash';

import { CellType } from '#core/components/cell/constants';

import type { ListViewConfig } from '#core/components/views/types';
import type { PhaseConfig, PhaseEntityConfig } from '#core/types/common/studio-types';
import type { DeepPartial } from '#core/types/utility-types';

/**
 * Factory for creating PhaseEntityConfig test fixtures.
 * Provides minimal required properties for testing with sensible defaults.
 *
 * @param base - Partial object shared across all test fixtures for a test suite
 * @returns Function that generates a complete entity config using overrides.
 *
 * @example
 * ```typescript
 * const buildEntity = buildEntityConfigFactory();
 * const pipelineEntity = buildEntity({
 *   id: 'pipelines',
 *   service: 'pipeline'
 * });
 * ```
 */
export const buildEntityConfigFactory = (base: DeepPartial<PhaseEntityConfig> = {}) => {
  return (overrides: DeepPartial<PhaseEntityConfig> = {}): PhaseEntityConfig => {
    const required: PhaseEntityConfig = {
      id: 'default-entity',
      name: 'Default Entity',
      service: 'default',
      state: 'active',
      views: [
        {
          type: 'list',
          columns: [
            { id: 'metadata.name', label: 'Name', type: CellType.TEXT },
            { id: 'status', label: 'Status', type: CellType.TEXT },
          ],
        },
      ],
    };

    const result = merge({}, required, base, overrides);

    // Handle views array specifically to ensure empty arrays override defaults
    if ('views' in overrides) {
      result.views = overrides.views as ListViewConfig[];
    }

    return result;
  };
};

/**
 * Factory for creating PhaseConfig test fixtures.
 * Provides minimal required properties for testing with sensible defaults.
 *
 * @param base - Partial object shared across all test fixtures for a test suite
 * @returns Function that generates a complete phase config using overrides.
 *
 * @example
 * ```typescript
 * const buildPhase = buildPhaseConfigFactory();
 * const trainPhase = buildPhase({
 *   id: 'train',
 *   entities: [pipelineEntity, runEntity]
 * });
 * ```
 */
export const buildPhaseConfigFactory = (base: DeepPartial<PhaseConfig> = {}) => {
  return (overrides: DeepPartial<PhaseConfig> = {}): PhaseConfig => {
    const required: PhaseConfig = {
      id: 'default-phase',
      icon: 'default',
      name: 'Default Phase',
      state: 'active',
      entities: [],
    };

    return merge({}, required, base, overrides);
  };
};
