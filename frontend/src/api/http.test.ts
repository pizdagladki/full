import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { HttpClient } from './http';

describe('HttpClient', () => {
  const BASE = 'http://api.test';
  let client: HttpClient;

  beforeEach(() => {
    client = new HttpClient(BASE);
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // criterion: http-get — get sends GET to baseUrl + path and returns parsed JSON
  it('get sends GET to baseUrl + path and returns json', async () => {
    const data = { ok: true };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(data),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.get<typeof data>('/health');

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/health',
      expect.objectContaining({ method: 'GET' }),
    );
    expect(result).toEqual(data);
  });

  // criterion: http-post — post sends POST with JSON body and returns parsed JSON
  it('post sends POST with JSON body and returns json', async () => {
    const body = { name: 'test' };
    const response = { id: 1 };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(response),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.post<typeof response>('/items', body);

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/items',
      expect.objectContaining({ method: 'POST', body: JSON.stringify(body) }),
    );
    expect(result).toEqual(response);
  });

  // criterion: http-error — non-2xx response throws an error with status visible
  it('throws on non-2xx response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: () => Promise.resolve({}),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await expect(client.get('/missing')).rejects.toThrow('404');
  });

  // criterion: http-get — fails if get result is not returned (trivial-pass guard)
  it('get returns the parsed JSON body — fails if result is ignored', async () => {
    const data = { value: 42 };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(data),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.get<typeof data>('/value');
    // This fails if get returns undefined or the wrong shape
    expect(result.value).toBe(42);
  });
});
