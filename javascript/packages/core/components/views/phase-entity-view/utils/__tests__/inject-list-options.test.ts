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
    {
      name: 'revision with pipeline types scopes to Pipeline revisions and a type labelSelector',
      service: 'revision',
      pipelineTypes: ['batch', 'streaming'],
      expected: {
        fieldSelector: 'base_type=Pipeline',
        labelSelector: 'michelangelo/PipelineType in (batch,streaming)',
      },
    },
    {
      name: 'revision with no pipeline types still scopes to Pipeline revisions',
      service: 'revision',
      pipelineTypes: undefined,
      expected: { fieldSelector: 'base_type=Pipeline' },
    },
  ])('$name', ({ service, pipelineTypes, expected }) => {
    expect(injectListOptions(service, pipelineTypes)).toEqual(expected);
  });
});
