import { describe, expect, it, vi } from 'vitest';

vi.mock('../services', () => ({
  getServices: vi.fn(),
}));

const { getServices } = await import('../services');
const { getRpcHandlers } = await import('../handlers');
const mockGetServices = getServices as ReturnType<typeof vi.fn>;

// Single shared service mock — handlers are cached at module scope, so all
// three test cases share the same handler instances built from this one mock.
const createPipelineRun = vi.fn().mockResolvedValue({});
const updatePipelineRun = vi.fn().mockResolvedValue({});
const updateTriggerRun = vi.fn().mockResolvedValue({});

mockGetServices.mockResolvedValue(
  // Proxy returns vi.fn() for any unaccessed service method, so the handlers
  // module's full sweep of services at construction time doesn't crash.
  new Proxy({} as Record<string, Record<string, unknown>>, {
    get(_, serviceName: string) {
      if (serviceName === 'PipelineRunService') {
        return { createPipelineRun, updatePipelineRun };
      }
      if (serviceName === 'TriggerRunService') {
        return { updateTriggerRun };
      }
      return new Proxy({}, { get: () => vi.fn().mockResolvedValue({}) });
    },
  })
);

describe('rpc handlers — mutation envelope wrapping', () => {
  it('CreatePipelineRun wraps the bare record in a pipelineRun envelope', async () => {
    const handlers = await getRpcHandlers();
    const record = { metadata: { name: 'test-run' } };
    await handlers.CreatePipelineRun(record as never);

    expect(createPipelineRun).toHaveBeenCalledWith({ pipelineRun: record });
  });

  it('UpdatePipelineRun wraps the bare record in a pipelineRun envelope', async () => {
    const handlers = await getRpcHandlers();
    const record = { metadata: { name: 'updated-run' } };
    await handlers.UpdatePipelineRun(record as never);

    expect(updatePipelineRun).toHaveBeenCalledWith({ pipelineRun: record });
  });

  it('UpdateTriggerRun wraps the bare record in a triggerRun envelope', async () => {
    const handlers = await getRpcHandlers();
    const record = { metadata: { name: 'kill-target' } };
    await handlers.UpdateTriggerRun(record as never);

    expect(updateTriggerRun).toHaveBeenCalledWith({ triggerRun: record });
  });
});
