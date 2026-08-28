import { PIPELINE_DETAIL_CONFIG } from '../detail';

import type { TableDetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';

describe('PIPELINE_DETAIL_CONFIG', () => {
  const runsPage = PIPELINE_DETAIL_CONFIG.pages?.find(
    (p): p is TableDetailPageConfig => p.id === 'runs' && p.type === 'table'
  );

  it('has a Runs page', () => {
    expect(runsPage).toBeDefined();
  });

  it('queries the pipelineRun service', () => {
    expect(runsPage?.queryConfig.service).toBe('pipelineRun');
  });

  it('filters by fieldSelector on pipeline_name, not a labelSelector', () => {
    const listOptions = (
      runsPage?.queryConfig.serviceOptions as Record<string, unknown> | undefined
    )?.listOptions as Record<string, string> | undefined;

    expect(listOptions).toBeDefined();
    expect(listOptions?.fieldSelector).toBe('pipeline_name=${page.metadata.name}');
    // The previous labelSelector on pipeline.michelangelo/name was written by
    // nothing in the platform and always returned zero results.
    expect(listOptions?.labelSelector).toBeUndefined();
  });
});
