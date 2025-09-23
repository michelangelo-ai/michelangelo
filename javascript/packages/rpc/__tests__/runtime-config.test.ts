import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getRuntimeConfig } from '../runtime-config';

// Mock fetch globally
global.fetch = vi.fn();
const mockFetch = fetch as ReturnType<typeof vi.fn>;

// Mock window.location
const mockLocation = {
  hostname: 'localhost',
};

Object.defineProperty(global, 'window', {
  value: { location: mockLocation },
  writable: true,
});

describe('getApiConfig', () => {
  beforeEach(() => {
    mockFetch.mockClear();
  });

  it('should return apiBaseUrl from config.json when fetch succeeds', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ apiBaseUrl: 'http://production-envoy:8081' }),
    } as Response);

    const result = await getRuntimeConfig();

    expect(result.apiBaseUrl).toBe('http://production-envoy:8081');
    expect(mockFetch).toHaveBeenCalledWith('/config.json');
  });

  it('should throw error when fetch fails with network error', async () => {
    mockFetch.mockRejectedValueOnce(new Error('Network error'));

    await expect(getRuntimeConfig()).rejects.toThrow(
      'Failed to load runtime configuration. Check that config.json is properly mounted.'
    );
  });

  it('should throw error when config.json returns 404', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
    } as Response);

    await expect(getRuntimeConfig()).rejects.toThrow(
      'Failed to load runtime configuration. Check that config.json is properly mounted.'
    );
  });

  it('should throw error when config.json returns 500', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
    } as Response);

    await expect(getRuntimeConfig()).rejects.toThrow(
      'Failed to load runtime configuration. Check that config.json is properly mounted.'
    );
  });

  it('should throw specific error when JSON parsing fails', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.reject(new SyntaxError('Unexpected token')),
    } as Response);

    await expect(getRuntimeConfig()).rejects.toThrow(
      'Failed to load runtime configuration. Check that config.json contains valid JSON.'
    );
  });

  it('should throw specific error when apiBaseUrl field is missing', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ someOtherField: 'value' }),
    } as Response);

    await expect(getRuntimeConfig()).rejects.toThrow(
      'Failed to load runtime configuration. Check that config.json contains apiBaseUrl field.'
    );
  });

  it('should throw specific error for network connectivity issues', async () => {
    mockFetch.mockRejectedValueOnce(new TypeError('Failed to fetch'));

    await expect(getRuntimeConfig()).rejects.toThrow(
      'Failed to load runtime configuration. Check network connectivity.'
    );
  });
});
