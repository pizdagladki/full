const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface MatchEntry {
  match_id: string;
  opponent_id: string;
  opponent_name?: string;
  result: 'win' | 'loss';
  mode: string;
  elo_delta: number;
  duration_ms: number;
  played_at: string; // ISO 8601
}

export interface MatchHistoryApi {
  getMatchHistory(): Promise<MatchEntry[]>;
}

export class MatchHistoryApiClient implements MatchHistoryApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async getMatchHistory(): Promise<MatchEntry[]> {
    const res = await fetch(`${this.baseUrl}/v1/matches/history`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(`GET /v1/matches/history failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<MatchEntry[]>;
  }
}

export const defaultMatchHistoryApi: MatchHistoryApi = new MatchHistoryApiClient();
