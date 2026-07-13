import { describe, expect, it, vi } from 'vitest';

import { getRpcHandlers } from '../handlers';

const mockGetServices = vi.hoisted(() => vi.fn());
vi.mock('../services', () => ({ getServices: mockGetServices }));

const createPipelineRun = vi.fn().mockResolvedValue({});
const updatePipelineRun = vi.fn().mockResolvedValue({});
const updateTriggerRun = vi.fn().mockResolvedValue({});

mockGetServices.mockResolvedValue(
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

    expect(createPipelineRun).toHaveBeenCalledWith({ pipelineRun: record }, undefined);
  });

  it('UpdatePipelineRun wraps the bare record in a pipelineRun envelope', async () => {
    const handlers = await getRpcHandlers();
    const record = { metadata: { name: 'updated-run' } };
    await handlers.UpdatePipelineRun(record as never);

    expect(updatePipelineRun).toHaveBeenCalledWith({ pipelineRun: record }, undefined);
  });

  it('UpdateTriggerRun wraps the bare record in a triggerRun envelope', async () => {
    const handlers = await getRpcHandlers();
    const record = { metadata: { name: 'kill-target' } };
    await handlers.UpdateTriggerRun(record as never);

    expect(updateTriggerRun).toHaveBeenCalledWith({ triggerRun: record }, undefined);
  });

  it('CreatePipelineRun stamps spec.actor from x-user-name header', async () => {
    const handlers = await getRpcHandlers();
    const record = { metadata: { name: 'run-1' }, spec: { pipeline: { name: 'p1' } } };
    await handlers.CreatePipelineRun(record as never, { 'x-user-name': 'jane' });

    expect(record.spec).toHaveProperty('actor');
    expect((record.spec as Record<string, unknown>).actor).toMatchObject({ name: 'jane' });
  });

  it('CreatePipelineRun leaves spec.actor unset when x-user-name header is absent', async () => {
    const handlers = await getRpcHandlers();
    const record = { metadata: { name: 'run-2' }, spec: { pipeline: { name: 'p1' } } };
    await handlers.CreatePipelineRun(record as never);

    expect((record.spec as Record<string, unknown>).actor).toBeUndefined();
  });
});
