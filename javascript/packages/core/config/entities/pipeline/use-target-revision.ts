import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { buildRevisionName, getLatestRevisionRef } from '#core/utils/revision-utils';

import type { RevisionRef } from '#core/types/common/studio-types';
import type { Pipeline } from './types';

/**
 * The Revision a run started from the current page should pin to.
 *
 * `?revisionId=` names it when a specific revision is being viewed; otherwise it defaults
 * to the latestRevision of the pipeline.
 */
export function useTargetRevision(record: Pipeline | undefined): RevisionRef | undefined {
  const { projectId, revisionId } = useStudioParams('base');
  const pipelineName = record?.metadata?.name;

  if (revisionId && pipelineName) {
    return { name: buildRevisionName('pipeline', pipelineName, revisionId), namespace: projectId };
  }
  return getLatestRevisionRef(record);
}
