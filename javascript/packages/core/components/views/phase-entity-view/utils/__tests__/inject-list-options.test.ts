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
      name: 'pipeline with pipeline types builds a fieldSelector',
      service: 'pipeline',
      pipelineTypes: ['batch', 'streaming'],
      expected: { fieldSelector: 'pipeline_type in (batch,streaming)' },
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
      name: 'pipelineRun is unfiltered even with pipeline types',
      service: 'pipelineRun',
      pipelineTypes: ['batch'],
      expected: undefined,
    },
    {
      name: 'triggerRun is unfiltered even with pipeline types',
      service: 'triggerRun',
      pipelineTypes: ['batch'],
      expected: undefined,
    },
  ])('$name', ({ service, pipelineTypes, expected }) => {
    expect(injectListOptions(service, pipelineTypes)).toEqual(expected);
  });
});
