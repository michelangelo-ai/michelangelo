import { createQueryMockRouter } from '#core/test/wrappers/get-service-provider-wrapper';

describe('createQueryMockRouter', () => {
  test('routes different query names to correct responses', async () => {
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: { pipelineRun: { name: 'specific-run' } },
      ListPipelineRun: { pipelineRunList: { items: [{ name: 'list-item' }] } },
      GetDataset: { dataset: { name: 'dataset-name' } },
    });

    const runResponse = await mockRequest('GetPipelineRun', { name: 'run-123' });
    const listResponse = await mockRequest('ListPipelineRun', {});
    const datasetResponse = await mockRequest('GetDataset', { namespace: 'test' });

    expect(runResponse).toEqual({ pipelineRun: { name: 'specific-run' } });
    expect(listResponse).toEqual({ pipelineRunList: { items: [{ name: 'list-item' }] } });
    expect(datasetResponse).toEqual({ dataset: { name: 'dataset-name' } });
  });

  test('rejects unmocked queries with helpful error messages', async () => {
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: { pipelineRun: { name: 'test' } },
    });

    await expect(mockRequest('UnmockedQuery', { filter: 'active' })).rejects.toThrow(
      'Unexpected query: UnmockedQuery with args: {"filter":"active"}'
    );
  });

  test('matches queries with specific service options', async () => {
    const mockRequest = createQueryMockRouter({
      'ListPipelineRun:{"filter":"status=SUCCESS","limit":10,"namespace":"project"}': {
        pipelineRunList: { items: [{ name: 'filtered-run' }] },
      },
      ListPipelineRun: {
        pipelineRunList: { items: [{ name: 'default-run' }] },
      },
    });

    const filteredResponse = await mockRequest('ListPipelineRun', {
      filter: 'status=SUCCESS',
      limit: 10,
      namespace: 'project',
    });
    const defaultResponse = await mockRequest('ListPipelineRun', {});

    expect(filteredResponse).toEqual({
      pipelineRunList: { items: [{ name: 'filtered-run' }] },
    });
    expect(defaultResponse).toEqual({
      pipelineRunList: { items: [{ name: 'default-run' }] },
    });
  });

  test('matches service options regardless of property order', async () => {
    const mockRequest = createQueryMockRouter({
      'ListPipelineRun:{"filter":"active","limit":20,"namespace":"test-project"}': {
        pipelineRunList: { items: [{ name: 'order-agnostic-match' }] },
      },
    });

    const response = await mockRequest('ListPipelineRun', {
      limit: 20,
      namespace: 'test-project',
      filter: 'active',
    });

    expect(response).toEqual({
      pipelineRunList: { items: [{ name: 'order-agnostic-match' }] },
    });
  });

  test('handles complex nested service options', async () => {
    const mockRequest = createQueryMockRouter({
      'GetPipelineRun:{"metadata":{"labels":{"env":"prod"}},"namespace":"production"}': {
        pipelineRun: { name: 'production-run' },
      },
    });

    const response = await mockRequest('GetPipelineRun', {
      metadata: { labels: { env: 'prod' } },
      namespace: 'production',
    });

    expect(response).toEqual({ pipelineRun: { name: 'production-run' } });
  });

  test('falls back to basic query name when specific args do not match', async () => {
    const mockRequest = createQueryMockRouter({
      'ListPipelineRun:{"filter":"status=SUCCESS"}': {
        pipelineRunList: { items: [{ name: 'success-only' }] },
      },
      ListPipelineRun: {
        pipelineRunList: { items: [{ name: 'fallback-response' }] },
      },
    });

    const response = await mockRequest('ListPipelineRun', { filter: 'status=FAILED' });

    expect(response).toEqual({
      pipelineRunList: { items: [{ name: 'fallback-response' }] },
    });
  });

  test('properly rejects with Error objects', async () => {
    const mockRequest = createQueryMockRouter({
      GetPipelineRun: new Error('Pipeline not found'),
      ListPipelineRun: { pipelineRunList: { items: [] } },
    });

    await expect(mockRequest('GetPipelineRun', { name: 'missing-run' })).rejects.toThrow(
      'Pipeline not found'
    );

    const successResponse = await mockRequest('ListPipelineRun', {});
    expect(successResponse).toEqual({ pipelineRunList: { items: [] } });
  });

  test('handles error responses with specific service options', async () => {
    const mockRequest = createQueryMockRouter({
      'GetPipelineRun:{"name":"restricted"}': new Error('Access denied'),
      GetPipelineRun: { pipelineRun: { name: 'default-access' } },
    });

    await expect(mockRequest('GetPipelineRun', { name: 'restricted' })).rejects.toThrow(
      'Access denied'
    );

    const successResponse = await mockRequest('GetPipelineRun', { name: 'public' });
    expect(successResponse).toEqual({ pipelineRun: { name: 'default-access' } });
  });

  test('handles empty args object', async () => {
    const mockRequest = createQueryMockRouter({
      'ListPipelineRun:{}': { pipelineRunList: { items: [{ name: 'empty-args' }] } },
      ListPipelineRun: { pipelineRunList: { items: [{ name: 'no-args' }] } },
    });

    const emptyArgsResponse = await mockRequest('ListPipelineRun', {});
    expect(emptyArgsResponse).toEqual({ pipelineRunList: { items: [{ name: 'empty-args' }] } });
  });
});
