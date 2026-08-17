import type { QueryConfig } from '#core/types/query-types';
import type { InjectedListOptions } from '../types';

/**
 * Derives the RPC-level listOptions needed to restrict an entity's list query to a
 * phase's pipeline types. Only `pipeline` is handled today: PipelineRun/TriggerRun have
 * no way to filter by pipeline type server-side. Two candidate mechanisms were checked
 * and both are dead ends:
 * - `michelangelo/SourcePipelineType` label: never stamped onto PipelineRun/TriggerRun
 *   rows (only read for notification formatting — go/base/notification/types/types.go).
 * - `pipelinerun.michelangelo/pipeline-type` label: read in
 *   go/components/pipelinerun/controller.go's getPipelineType(), but never written
 *   anywhere — that function is a metrics-labeling stub that always falls through to
 *   "unknown".
 * PipelineRunSpec/TriggerRunSpec also only reference the target Pipeline by name
 * (spec.pipeline), not by type, and the storage layer's List() resolves field/label
 * selectors against a single resource's own indexed columns — there's no join back to
 * the referenced Pipeline's `spec.type`. Filtering these two services requires
 * denormalizing pipeline type onto PipelineRun/TriggerRun at creation time (proto +
 * migration), which is a backend change out of scope here. Returns undefined when
 * there's nothing to inject.
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
