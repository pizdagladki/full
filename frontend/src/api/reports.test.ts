import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ReportsApiClient } from './reports';

describe('ReportsApiClient', () => {
  const BASE = 'http://api.test';
  let client: ReportsApiClient;

  beforeEach(() => {
    client = new ReportsApiClient(BASE);
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // ---------------------------------------------------------------------------
  // reportCheat
  // ---------------------------------------------------------------------------

  // criterion: 4 — "Report cheating" calls POST /v1/reports/cheat with {reported_id, match_id}.
  it('reportCheat sends POST /v1/reports/cheat with reported_id and match_id, credentials included', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await client.reportCheat({ reported_id: 42, match_id: 'match-7' });

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/v1/reports/cheat',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reported_id: 42, match_id: 'match-7' }),
      }),
    );
  });

  // criterion: 4 (violation guard) — a non-2xx response from the cheat report must throw, not
  // silently resolve as if the report succeeded.
  it('reportCheat throws on a non-2xx response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await expect(client.reportCheat({ reported_id: 42, match_id: 'match-7' })).rejects.toThrow(
      'POST /v1/reports/cheat failed: 500 Internal Server Error',
    );
  });

  // ---------------------------------------------------------------------------
  // reportBug — mobile (text-only JSON) path
  // ---------------------------------------------------------------------------

  // criterion: 4 — "Report a bug" on mobile sends a JSON body (text-only, no recording attached).
  it('reportBug (mobile) sends a JSON POST /v1/reports/bug with device + description', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await client.reportBug({ device: 'mobile', description: 'Screen froze' });

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/v1/reports/bug',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ device: 'mobile', description: 'Screen froze' }),
      }),
    );
  });

  // criterion: 4 (violation guard) — a non-2xx response from the mobile bug report must throw.
  it('reportBug (mobile) throws on a non-2xx response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await expect(client.reportBug({ device: 'mobile', description: 'oops' })).rejects.toThrow(
      'POST /v1/reports/bug failed: 400 Bad Request',
    );
  });

  // ---------------------------------------------------------------------------
  // reportBug — PC (multipart, with recording) path
  // ---------------------------------------------------------------------------

  // criterion: 4 — on PC, a recording MAY be attached; when it is, the request is sent as
  // multipart/form-data (no explicit Content-Type header — the browser sets the boundary).
  it('reportBug (pc, with recording) sends a multipart FormData POST /v1/reports/bug', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const recording = new Blob(['fake-webm-bytes'], { type: 'video/webm' });
    await client.reportBug({ device: 'pc', description: 'Crash on rematch', recording });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('http://api.test/v1/reports/bug');
    expect(init.method).toBe('POST');
    expect(init.credentials).toBe('include');
    expect(init.body).toBeInstanceOf(FormData);
    const form = init.body as FormData;
    expect(form.get('device')).toBe('pc');
    expect(form.get('description')).toBe('Crash on rematch');
    // jsdom's FormData wraps an appended Blob as a File — assert on its Blob-ness/contents
    // instead of reference equality.
    const recordingEntry = form.get('recording');
    expect(recordingEntry).toBeInstanceOf(Blob);
    expect((recordingEntry as Blob).size).toBe(recording.size);
    expect((recordingEntry as Blob).type).toBe(recording.type);
  });

  // criterion: 4 (violation guard) — a non-2xx response from the PC (multipart) bug report must
  // throw too — the recording path must not silently swallow a server-side failure.
  it('reportBug (pc, with recording) throws on a non-2xx response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const recording = new Blob(['x'], { type: 'video/webm' });
    await expect(client.reportBug({ device: 'pc', recording })).rejects.toThrow(
      'POST /v1/reports/bug failed: 500 Internal Server Error',
    );
  });
});
