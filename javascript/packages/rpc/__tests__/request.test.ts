import { expect, it, vi } from 'vitest';

import { request } from '../request';

vi.mock('../handlers', () => ({
  getRpcHandlers: vi.fn(),
}));

const { getRpcHandlers } = await import('../handlers');
const mockGetRpcHandlers = getRpcHandlers as ReturnType<typeof vi.fn>;

function mockHandler(response: unknown) {
  mockGetRpcHandlers.mockResolvedValue({
    GetPipelineRun: vi.fn().mockResolvedValue(response),
  });
}

it('strips $typeName and $unknown from response', async () => {
  mockHandler({ $typeName: 'foo.Bar', $unknown: [], name: 'test' });

  expect(await request('GetPipelineRun', {} as never)).toEqual({ name: 'test' });
});

it('recursively strips protobuf internals from nested objects', async () => {
  mockHandler({
    $typeName: 'outer',
    nested: { $typeName: 'inner', value: 1 },
  });

  expect(await request('GetPipelineRun', {} as never)).toEqual({ nested: { value: 1 } });
});

it('preserves Uint8Array fields without corrupting them into plain objects', async () => {
  const bytes = new Uint8Array([1, 2, 3]);
  mockHandler({ $typeName: 'foo.Any', typeUrl: 'type.googleapis.com/foo', value: bytes });

  const result = await request('GetPipelineRun', {} as never);

  expect((result as { value: Uint8Array }).value).toBeInstanceOf(Uint8Array);
  expect((result as { value: Uint8Array }).value).toEqual(bytes);
});

it('handles arrays containing objects with protobuf internals', async () => {
  mockHandler({
    items: [
      { $typeName: 'foo', x: 1 },
      { $typeName: 'bar', x: 2 },
    ],
  });

  expect(await request('GetPipelineRun', {} as never)).toEqual({ items: [{ x: 1 }, { x: 2 }] });
});
