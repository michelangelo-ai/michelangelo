import { create } from '@bufbuild/protobuf';
import { anyPack, StringValueSchema } from '@bufbuild/protobuf/wkt';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { request } from '../request';

// Bypass the /config.json fetch — we only care about the RPC transport layer.
vi.mock('../runtime-config', () => ({
  getRuntimeConfig: () => Promise.resolve({ apiBaseUrl: 'http://test' }),
}));

// The real createConnectTransport (JSON mode) calls response.json() and decodes
// it with fromJson(..., jsonOptions). If jsonOptions.registry doesn't include
// TypedStructSchema, fromJson throws on the @type URL — so removing the registry
// from services.ts breaks this test.
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

  // The registry decodes the Any to a binary TypedStruct, and toPlainObject unpacks it to
  // { typeUrl, value } where typeUrl names the inner config type and value is its plain
  // JSON. Without TypedStructSchema in the registry, fromJson throws before reaching here.
  expect(details[0]).toEqual({
    typeUrl: 'type.googleapis.com/michelangelo.UniFlowConf',
    value: {},
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

  it('sends an already-packed Any (real typeUrl/value) through unchanged', async () => {
    const realAny = anyPack(
      StringValueSchema,
      create(StringValueSchema, { value: 'already-packed' })
    );

    await request('ListPipelineRun', {
      listOptionsExt: {
        operation: { criterion: [{ fieldName: 'x', operator: 1, matchValue: realAny }] },
      },
    } as never);

    expectCriteria({
      fieldName: 'x',
      operator: 'CRITERION_OPERATOR_EQUAL',
      matchValue: {
        '@type': 'type.googleapis.com/google.protobuf.StringValue',
        value: 'already-packed',
      },
    });
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
