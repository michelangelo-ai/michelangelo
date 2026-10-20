import { beforeEach, describe, expect, it, vi } from 'vitest';

import { request } from '../request';

// Bypass the /config.json fetch — we only care about the RPC transport layer.
vi.mock('../runtime-config', () => ({
  getRuntimeConfig: () => Promise.resolve({ apiBaseUrl: 'http://test' }),
}));

global.fetch = vi.fn().mockResolvedValue({
  status: 200,
  headers: new Headers({ 'content-type': 'application/json' }),
  json: () =>
    Promise.resolve({
      pipelineRunList: {
        items: [
          {
            status: {
              details: [
                {
                  '@type': 'type.googleapis.com/michelangelo.api.TypedStruct',
                  typeUrl: 'type.googleapis.com/michelangelo.UniFlowConf',
                  value: {},
                },
              ],
            },
          },
        ],
      },
    }),
});

it('decodes a ListPipelineRun response containing a TypedStruct Any field', async () => {
  const result = await request('ListPipelineRun', {} as never);
  const details = (
    result as unknown as { pipelineRunList: { items: { status: { details: unknown[] } }[] } }
  ).pipelineRunList.items[0].status.details;

  // Responses are returned as Envoy emits them, so the Any comes through as-is with its '@type'.
  expect(details[0]).toMatchObject({
    '@type': 'type.googleapis.com/michelangelo.api.TypedStruct',
    typeUrl: 'type.googleapis.com/michelangelo.UniFlowConf',
  });
});

// Verifies the Any lands on the wire in the shape Envoy's grpc_json_transcoder expects.
describe('outgoing request — Any-packing through the real service client', () => {
  beforeEach(() => {
    vi.mocked(global.fetch).mockClear();
  });

  function expectCriteria(
    ...expected: Array<{ fieldName: string; operator: string; matchValue: unknown }>
  ) {
    const calls = vi.mocked(global.fetch).mock.calls;
    const [, init] = calls.at(-1) as [string, RequestInit];
    // cast: this test only cares about the shape it itself constructed
    const body = JSON.parse(init.body as string) as {
      listOptionsExt: { operation: { criterion: Record<string, unknown>[] } };
    };
    const criteria = body.listOptionsExt.operation.criterion;
    expect(criteria).toHaveLength(expected.length);
    for (const [i, criterion] of expected.entries()) {
      expect(criteria[i]).toMatchObject(criterion);
    }
  }

  it('packs a string matchValue into a StringValue Any', async () => {
    await request('ListPipelineRun', {
      listOptionsExt: {
        operation: {
          criterion: [
            { fieldName: 'pipeline_run.pipeline_name', operator: 1, matchValue: 'my-pipeline' },
          ],
        },
      },
    } as never);

    expectCriteria({
      fieldName: 'pipeline_run.pipeline_name',
      operator: 'CRITERION_OPERATOR_EQUAL',
      matchValue: {
        '@type': 'type.googleapis.com/google.protobuf.StringValue',
        value: 'my-pipeline',
      },
    });
  });

  it('packs a boolean matchValue into a BoolValue Any', async () => {
    await request('ListPipelineRun', {
      listOptionsExt: {
        operation: { criterion: [{ fieldName: 'x', operator: 1, matchValue: true }] },
      },
    } as never);

    expectCriteria({
      fieldName: 'x',
      operator: 'CRITERION_OPERATOR_EQUAL',
      matchValue: { '@type': 'type.googleapis.com/google.protobuf.BoolValue', value: true },
    });
  });

  it('packs an integer matchValue into an Int64Value Any', async () => {
    await request('ListPipelineRun', {
      listOptionsExt: {
        operation: { criterion: [{ fieldName: 'x', operator: 1, matchValue: 42 }] },
      },
    } as never);

    expectCriteria({
      fieldName: 'x',
      operator: 'CRITERION_OPERATOR_EQUAL',
      matchValue: { '@type': 'type.googleapis.com/google.protobuf.Int64Value', value: '42' },
    });
  });

  it('packs a float matchValue into a DoubleValue Any', async () => {
    await request('ListPipelineRun', {
      listOptionsExt: {
        operation: { criterion: [{ fieldName: 'x', operator: 1, matchValue: 1.5 }] },
      },
    } as never);

    expectCriteria({
      fieldName: 'x',
      operator: 'CRITERION_OPERATOR_EQUAL',
      matchValue: { '@type': 'type.googleapis.com/google.protobuf.DoubleValue', value: 1.5 },
    });
  });

  it('sends an already-packed Any (proto3 JSON @type form) through unchanged', async () => {
    const packedAny = {
      '@type': 'type.googleapis.com/google.protobuf.StringValue',
      value: 'already-packed',
    };

    await request('ListPipelineRun', {
      listOptionsExt: {
        operation: { criterion: [{ fieldName: 'x', operator: 1, matchValue: packedAny }] },
      },
    } as never);

    expectCriteria({ fieldName: 'x', operator: 'CRITERION_OPERATOR_EQUAL', matchValue: packedAny });
  });

  it('packs Any values across every entry of a repeated field, and leaves fieldName untouched', async () => {
    await request('ListPipelineRun', {
      listOptionsExt: {
        operation: {
          criterion: [
            { fieldName: 'a', operator: 1, matchValue: 'one' },
            { fieldName: 'b', operator: 1, matchValue: 'two' },
          ],
        },
      },
    } as never);

    expectCriteria(
      {
        fieldName: 'a',
        operator: 'CRITERION_OPERATOR_EQUAL',
        matchValue: { '@type': 'type.googleapis.com/google.protobuf.StringValue', value: 'one' },
      },
      {
        fieldName: 'b',
        operator: 'CRITERION_OPERATOR_EQUAL',
        matchValue: { '@type': 'type.googleapis.com/google.protobuf.StringValue', value: 'two' },
      }
    );
  });

  // Rejects rather than letting create() silently turn this into an empty, corrupted Any —
  // callers already normalize thrown RPC errors, so this surfaces as a normal query error.
  it('rejects when an Any field is given a value with no wrapper mapping', async () => {
    await expect(
      request('ListPipelineRun', {
        listOptionsExt: {
          operation: {
            criterion: [{ fieldName: 'x', operator: 1, matchValue: { nested: 'object' } }],
          },
        },
      } as never)
    ).rejects.toThrow(/cannot auto-pack object/);
  });

  // A map<string, Any> field on an unrelated service/message, proving the packing is
  // schema-driven rather than special-cased for Criterion.
  it('packs a map<string, Any> field on CreateDeployment, an unrelated service method', async () => {
    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: () => Promise.resolve({}),
    });

    await request('CreateDeployment', {
      metadata: { name: 'my-deployment' },
      status: { providerStatus: { foo: 'bar-value', replicas: 3 } },
    } as never);

    const calls = (global.fetch as ReturnType<typeof vi.fn>).mock.calls;
    const [, init] = calls.at(-1) as [string, RequestInit];
    const body = JSON.parse(init.body as string) as {
      deployment: { status: { providerStatus: Record<string, unknown> } };
    };

    expect(body.deployment.status.providerStatus.foo).toEqual({
      '@type': 'type.googleapis.com/google.protobuf.StringValue',
      value: 'bar-value',
    });
    expect(body.deployment.status.providerStatus.replicas).toEqual({
      '@type': 'type.googleapis.com/google.protobuf.Int64Value',
      value: '3',
    });
  });
});

describe('outgoing request — proto3 JSON input', () => {
  function lastRequestBody() {
    const [, init] = vi.mocked(global.fetch).mock.calls.at(-1) as [string, RequestInit];
    // cast: these tests only care about the shape they themselves constructed
    return JSON.parse(init.body as string) as Record<string, Record<string, unknown>>;
  }

  it('accepts string enum names', async () => {
    await request('CreateInferenceServer', {
      metadata: { name: 'my-server' },
      spec: { backendType: 'BACKEND_TYPE_TRITON' },
    } as never);

    expect(lastRequestBody().inferenceServer.spec).toMatchObject({
      backendType: 'BACKEND_TYPE_TRITON',
    });
  });

  it('accepts a oneof member set directly on its parent', async () => {
    await request('CreateDeployment', {
      metadata: { name: 'my-deployment' },
      spec: { inferenceServer: { name: 'my-server', namespace: 'my-project' } },
    } as never);

    expect(lastRequestBody().deployment.spec).toMatchObject({
      inferenceServer: { name: 'my-server', namespace: 'my-project' },
    });
  });

  it('ignores keys that are not fields of the request message', async () => {
    await request('CreateInferenceServer', {
      metadata: { name: 'my-server' },
      notAField: true,
    } as never);

    expect(lastRequestBody().inferenceServer).not.toHaveProperty('notAField');
  });

  it('drops metadata.deletionGracePeriodSeconds, including inside Any payloads', async () => {
    await request('UpdatePipelineRun', {
      metadata: { name: 'run', deletionGracePeriodSeconds: '0' },
      status: {
        details: [
          {
            '@type': 'type.googleapis.com/michelangelo.api.v2.Pipeline',
            metadata: { name: 'pipeline', deletionGracePeriodSeconds: '0' },
          },
        ],
      },
    } as never);

    const { pipelineRun } = lastRequestBody();
    expect(pipelineRun.metadata).toEqual({ name: 'run' });
    expect(pipelineRun.status).toEqual({
      details: [
        {
          '@type': 'type.googleapis.com/michelangelo.api.v2.Pipeline',
          metadata: { name: 'pipeline' },
        },
      ],
    });
  });
});

describe('Timestamp/Duration fields', () => {
  function respondWith(json: unknown) {
    vi.mocked(global.fetch).mockResolvedValueOnce({
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: () => Promise.resolve(json),
    } as Response);
  }

  it('arrive as proto3 JSON strings', async () => {
    const pipelineRun = {
      metadata: { creationTimestamp: '2026-09-24T23:07:46Z' },
      status: { steps: [{ startTime: '2026-09-28T23:11:19.992009180Z' }] },
    };
    respondWith({ pipelineRun });

    const response = await request('GetPipelineRun', {} as never);

    expect(response).toEqual({ pipelineRun });
  });

  it('are sent as proto3 JSON strings', async () => {
    respondWith({});

    await request('CreateTriggerRun', {
      metadata: { name: 'backfill' },
      spec: {
        startTimestamp: '2023-11-14T22:13:20Z',
        trigger: { intervalSchedule: { interval: '3600s' } },
      },
    } as never);

    const [, init] = vi.mocked(global.fetch).mock.calls.at(-1) as [string, RequestInit];
    // cast: this test only cares about the shape it itself constructed
    const body = JSON.parse(init.body as string) as {
      triggerRun: { spec: Record<string, unknown> };
    };
    expect(body.triggerRun.spec).toMatchObject({
      startTimestamp: '2023-11-14T22:13:20Z',
      trigger: { intervalSchedule: { interval: '3600s' } },
    });
  });
});
