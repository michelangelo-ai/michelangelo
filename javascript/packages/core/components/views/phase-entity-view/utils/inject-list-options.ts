import type { QueryConfig } from '#core/types/query-types';
import type { InjectedListOptions } from '../types';

const SOURCE_PIPELINE_TYPE_LABEL = 'michelangelo/SourcePipelineType';
const PIPELINE_TYPE_LABEL = 'michelangelo/PipelineType';

/**
 * Builds the server-side list scoping for a phase entity.
 */
export function injectListOptions(
  service: QueryConfig['service'],
  pipelineTypes?: string[]
): InjectedListOptions | undefined {
  const types = pipelineTypes?.length ? pipelineTypes.join(',') : undefined;

  if (service === 'revision') {
    return {
      fieldSelector: 'base_type=Pipeline',
      ...(types && { labelSelector: `${PIPELINE_TYPE_LABEL} in (${types})` }),
    };
  }

  if (!types) return undefined;

  if (service === 'pipeline') {
    return { fieldSelector: `pipeline_type in (${types})` };
  }

  if (service === 'pipelineRun' || service === 'triggerRun') {
    return { labelSelector: `${SOURCE_PIPELINE_TYPE_LABEL} in (${types})` };
  }

  return undefined;
}
