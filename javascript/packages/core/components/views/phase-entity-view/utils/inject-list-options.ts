import type { QueryConfig } from '#core/types/query-types';
import type { InjectedListOptions } from '../types';

/**
 * Derives the RPC-level listOptions needed to restrict an entity's list query to a
 * phase's pipeline types.
 */
export function injectListOptions(
  service: QueryConfig['service'],
  pipelineTypes?: string[]
): InjectedListOptions | undefined {
  if (!pipelineTypes?.length) return undefined;

  if (service === 'pipeline') {
    return {
      fieldSelector: `pipeline_type in (${pipelineTypes.join(',')})`,
    };
  }

  return undefined;
}
