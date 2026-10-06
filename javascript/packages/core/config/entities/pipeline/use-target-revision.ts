import { getLatestRevisionRef } from '#core/utils/revision-utils';
import { isPipelineRevision } from './types';

import type { RevisionRef } from '#core/types/common/studio-types';
import type { Pipeline, PipelineRevision } from './types';

/**
 * The Revision a run started from this record should pin to.
 *
 * A Revision record already names the exact revision being viewed — its own identity,
 * no lookup needed. A Pipeline record has no specific revision in view, so this falls
 * back to its `status.latestRevision` pointer.
 */
export function useTargetRevision(
  record: Pipeline | PipelineRevision | undefined
): RevisionRef | undefined {
  if (isPipelineRevision(record)) {
    return { name: record.metadata.name, namespace: record.metadata.namespace };
  }
  return getLatestRevisionRef(record);
}
