import type { QueryConfig } from '#core/types/query-types';
import type { InjectedListOptions } from '../types';

/**
 * Derives the RPC-level listOptions needed to restrict an entity's list query to a
 * phase's pipeline types. Only `pipeline` is handled today: studio-web's equivalent
 * also filters pipelineRun/triggerRun by a `michelangelo/SourcePipelineType` label, but
 * the OSS apiserver never stamps that label onto PipelineRun/TriggerRun rows (it's only
 * read for notification formatting — see go/base/notification/types/types.go), so
 * applying it here would silently return zero rows. Leave runs/triggers unfiltered
 * until a backend change adds that label. Returns undefined when there's nothing to
 * inject.
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

  return undefined;
}
