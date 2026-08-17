import type { QueryConfig } from '#core/types/query-types';
import type { InjectedListOptions } from '../types';

/**
 * Derives the RPC-level listOptions needed to restrict an entity's list query to a
 * phase's pipeline types, branching per entity service the way studio-web's list view
 * does (pipelines filter by field, everything downstream of a pipeline run filters by
 * label). Returns undefined when there's nothing to inject.
 */
export function injectListOptions(
  service: QueryConfig['service'],
  pipelineTypes?: string[]
): InjectedListOptions | undefined {
  if (!pipelineTypes?.length) return undefined;

  if (service === 'pipeline') {
    return {
      // Permissive-mode passthrough: apiserver's field selector parser treats this key
      // as a literal MySQL column (indexPathToKeyMaps is nil), so it must be the raw
      // `pipeline_type` column, not the proto path `spec.type`.
      fieldSelector: `pipeline_type in (${pipelineTypes.join(',')})`,
    };
  }

  if (service === 'pipelineRun' || service === 'triggerRun') {
    return {
      labelSelector: `michelangelo/SourcePipelineType in (${pipelineTypes.join(',')})`,
    };
  }

  return undefined;
}
