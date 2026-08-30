import { describe, expect, test } from 'vitest';

import { injectListOptions } from '../inject-list-options';

import type { QueryConfig } from '#core/types/query-types';

describe('injectListOptions', () => {
  test.each<{
    name: string;
    service: QueryConfig['service'];
    pipelineTypes?: string[];
    expected: ReturnType<typeof injectListOptions>;
  }>([
    {
      name: 'pipeline with pipeline types builds a labelSelector',
      service: 'pipeline',
      pipelineTypes: ['batch', 'streaming'],
      expected: { labelSelector: 'michelangelo/SourcePipelineType in (batch,streaming)' },
    },
    {
      name: 'pipeline with no pipeline types is undefined',
      service: 'pipeline',
      pipelineTypes: undefined,
      expected: undefined,
    },
    {
      name: 'pipeline with empty pipeline types is undefined',
      service: 'pipeline',
      pipelineTypes: [],
      expected: undefined,
    },
    {
      name: 'pipelineRun with pipeline types builds a labelSelector',
      service: 'pipelineRun',
      pipelineTypes: ['batch'],
      expected: { labelSelector: 'michelangelo/SourcePipelineType in (batch)' },
    },
    {
      name: 'triggerRun with pipeline types builds a labelSelector',
      service: 'triggerRun',
      pipelineTypes: ['batch'],
      expected: { labelSelector: 'michelangelo/SourcePipelineType in (batch)' },
    },
  ])('$name', ({ service, pipelineTypes, expected }) => {
    expect(injectListOptions(service, pipelineTypes)).toEqual(expected);
  });
});
