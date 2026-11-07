import { beforeEach, describe, expect, it, vi } from 'vitest';

import { createFetchTransport } from '../create-fetch-transport';

global.fetch = vi.fn();
const mockFetch = fetch as ReturnType<typeof vi.fn>;

function mockJsonResponse(status: number, body: unknown) {
  return {
    status,
    json: () => Promise.resolve(body),
  } as Response;
}

describe('createFetchTransport', () => {
  beforeEach(() => {
    mockFetch.mockClear();
  });

  it('omits the fetch credentials option when not configured', async () => {
    mockFetch.mockResolvedValueOnce(mockJsonResponse(200, { ok: true }));

    const transport = createFetchTransport({ baseUrl: 'https://api.example.com' });
    await transport.callUnary('FooService', 'GetFoo', {});

    const [, init] = mockFetch.mock.calls[0];
    expect(init).not.toHaveProperty('credentials');
  });

  it('passes credentials through to fetch when configured', async () => {
    mockFetch.mockResolvedValueOnce(mockJsonResponse(200, { ok: true }));

    const transport = createFetchTransport({
      baseUrl: 'https://api.example.com',
      credentials: 'include',
    });
    await transport.callUnary('FooService', 'GetFoo', {});

    const [, init] = mockFetch.mock.calls[0];
    expect(init).toMatchObject({ credentials: 'include' });
  });

  it('does not let a per-request header override the credentials mode', async () => {
    mockFetch.mockResolvedValueOnce(mockJsonResponse(200, { ok: true }));

    const transport = createFetchTransport({
      baseUrl: 'https://api.example.com',
      credentials: 'include',
    });
    await transport.callUnary('FooService', 'GetFoo', {}, { 'X-Extra': 'value' });

    const [, init] = mockFetch.mock.calls[0];
    expect(init).toMatchObject({ credentials: 'include' });
    expect((init as RequestInit).headers).toMatchObject({ 'X-Extra': 'value' });
  });

  it('still calls the expected URL and method with credentials configured', async () => {
    mockFetch.mockResolvedValueOnce(mockJsonResponse(200, { ok: true }));

    const transport = createFetchTransport({
      baseUrl: 'https://api.example.com/',
      credentials: 'include',
    });
    await transport.callUnary('FooService', 'GetFoo', { id: '1' });

    expect(mockFetch).toHaveBeenCalledWith(
      'https://api.example.com/FooService/GetFoo',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ id: '1' }) })
    );
  });
});
