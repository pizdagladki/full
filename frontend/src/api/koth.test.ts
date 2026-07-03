import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { KothApiClient } from './koth';

describe('KothApiClient', () => {
  const BASE = 'http://api.test';
  let client: KothApiClient;

  beforeEach(() => {
    client = new KothApiClient(BASE);
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // ---------------------------------------------------------------------------
  // getKing
  // ---------------------------------------------------------------------------

  // criterion: getKing-success — GET .../hills/:hillType/king returns the parsed KingInfo on 200.
  it('getKing sends GET to the hill king endpoint and returns the parsed body on 200', async () => {
    const king = { user_id: 1, clip_id: 'abc', blink_ts_ms: 500 };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(king),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.getKing('daily');

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/v1/koth/hills/daily/king',
      expect.objectContaining({ method: 'GET', credentials: 'include' }),
    );
    expect(result).toEqual(king);
  });

  // criterion: getKing-404 — a 404 (hill not seeded yet) resolves to null, not an error.
  it('getKing resolves to null on a 404 (hill not seeded yet)', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: () => Promise.resolve({}),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.getKing('monthly');

    expect(result).toBeNull();
  });

  // criterion: getKing-error — a non-404 non-ok response throws with the expected message.
  it('getKing throws on a non-ok, non-404 response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      statusText: 'Bad Request',
      json: () => Promise.resolve({}),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await expect(client.getKing('bogus')).rejects.toThrow(
      'GET /v1/koth/hills/bogus/king failed: 400 Bad Request',
    );
  });

  // ---------------------------------------------------------------------------
  // getRankedLeaderboard
  // ---------------------------------------------------------------------------

  // criterion: leaderboard-success — GET .../ranked/leaderboard returns the parsed array.
  it('getRankedLeaderboard sends GET to the leaderboard endpoint and returns the parsed array', async () => {
    const counts = [{ rank: 1, count: 3 }];
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(counts),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.getRankedLeaderboard();

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/v1/koth/ranked/leaderboard',
      expect.objectContaining({ method: 'GET', credentials: 'include' }),
    );
    expect(result).toEqual(counts);
  });

  // criterion: leaderboard-error — non-ok response throws with the expected message.
  it('getRankedLeaderboard throws on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: () => Promise.resolve({}),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await expect(client.getRankedLeaderboard()).rejects.toThrow(
      'GET /v1/koth/ranked/leaderboard failed: 500 Internal Server Error',
    );
  });

  // ---------------------------------------------------------------------------
  // getRankedMe
  // ---------------------------------------------------------------------------

  // criterion: me-success — GET .../ranked/me returns the parsed body.
  it('getRankedMe sends GET to the me endpoint and returns the parsed body', async () => {
    const me = { current_rank: 4, next_target_ms: 1000 };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(me),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const result = await client.getRankedMe();

    expect(fetchMock).toHaveBeenCalledWith(
      'http://api.test/v1/koth/ranked/me',
      expect.objectContaining({ method: 'GET', credentials: 'include' }),
    );
    expect(result).toEqual(me);
  });

  // criterion: me-error — non-ok response throws with the expected message.
  it('getRankedMe throws on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      statusText: 'Unauthorized',
      json: () => Promise.resolve({}),
    } as Response);
    vi.stubGlobal('fetch', fetchMock);

    await expect(client.getRankedMe()).rejects.toThrow(
      'GET /v1/koth/ranked/me failed: 401 Unauthorized',
    );
  });
});
