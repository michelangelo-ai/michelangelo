import { expect, it, vi } from 'vitest';

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

  // The Any is decoded to { typeUrl, value: Uint8Array } by the registry.
  // Without TypedStructSchema in the registry, fromJson throws before reaching here.
  expect(details[0]).toMatchObject({ typeUrl: 'type.googleapis.com/michelangelo.api.TypedStruct' });
  expect((details[0] as { value: unknown }).value).toBeInstanceOf(Uint8Array);
});
