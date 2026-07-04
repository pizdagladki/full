import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ClipsApiClient } from './clips';

describe('ClipsApiClient', () => {
  const BASE = 'http://api.test';
  let client: ClipsApiClient;

  beforeEach(() => {
    client = new ClipsApiClient(BASE);
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // ---------------------------------------------------------------------------
  // uploadClip
  // ---------------------------------------------------------------------------

  // criterion: 3 — a win-clip WebM blob is uploaded via POST /v1/clips, credentials included, and
  // a 201 response's {id} is parsed and returned to the caller.
  it('uploadClip sends POST /v1/clips with the blob body and returns the parsed id on 201', async () => {
    const blob = new Blob(['fake-webm-bytes'], { type: 'video/webm' });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: () => Promise.resolve({ id: 'clip-123' }),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.uploadClip(blob);

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/v1/clips',
      expect.objectContaining({ method: 'POST', credentials: 'include', body: blob }),
    );
    expect(result).toEqual({ id: 'clip-123' });
  });

  // criterion: 3 (violation guard) — a non-2xx response from the upload must throw, not silently
  // resolve with an undefined/garbage id.
  it('uploadClip throws on a non-2xx response', async () => {
    const blob = new Blob(['x'], { type: 'video/webm' });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: () => Promise.resolve({}),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await expect(client.uploadClip(blob)).rejects.toThrow(
      'POST /v1/clips failed: 500 Internal Server Error',
    );
  });

  // ---------------------------------------------------------------------------
  // convertClip
  // ---------------------------------------------------------------------------

  // criterion: 3 — MP4 conversion is requested via POST /v1/clips/:id/convert.
  it('convertClip sends POST /v1/clips/:id/convert with credentials included', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 202,
      json: () => Promise.resolve({}),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await client.convertClip('clip-123');

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/v1/clips/clip-123/convert',
      expect.objectContaining({ method: 'POST', credentials: 'include' }),
    );
  });

  // criterion: 3 (violation guard) — a non-2xx response from convert must throw.
  it('convertClip throws on a non-2xx response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: () => Promise.resolve({}),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await expect(client.convertClip('missing-clip')).rejects.toThrow(
      'POST /v1/clips/missing-clip/convert failed: 404 Not Found',
    );
  });
});
