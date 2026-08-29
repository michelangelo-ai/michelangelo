import type { QueryConfig } from '#core/types/query-types';
import type { InjectedListOptions } from '../types';

const SOURCE_PIPELINE_TYPE_LABEL = 'michelangelo/SourcePipelineType';

export function injectListOptions(
  service: QueryConfig['service'],
  pipelineTypes?: string[]
): InjectedListOptions | undefined {
  if (!pipelineTypes?.length) return undefined;

  if (service === 'pipeline' || service === 'pipelineRun' || service === 'triggerRun') {
    return {
      labelSelector: `${SOURCE_PIPELINE_TYPE_LABEL} in (${pipelineTypes.join(',')})`,
    };
  }

  return undefined;
}
