const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface ChallengeHillRequest {
  survived_ms: number;
  new_clip_id: string;
}

export interface KingInfo {
  user_id: number;
  clip_id: string;
  blink_ts_ms: number;
}

export interface ChallengeHillResult {
  won: boolean;
  king: KingInfo;
}

export interface RankedAttemptRequest {
  held_ms: number;
}

export interface RankedAttemptResult {
  achieved_rank: number;
  current_rank: number;
  newly_reached: boolean;
}

export interface RankCount {
  rank: number;
  count: number;
}

export interface RankedMeResult {
  current_rank: number;
  next_target_ms: number;
}

export interface KothApi {
  challengeHill(hillType: string, body: ChallengeHillRequest): Promise<ChallengeHillResult>;
  submitRankedAttempt(body: RankedAttemptRequest): Promise<RankedAttemptResult>;
  getKing(hillType: string): Promise<KingInfo | null>;
  getRankedLeaderboard(): Promise<RankCount[]>;
  getRankedMe(): Promise<RankedMeResult>;
}

export class KothApiClient implements KothApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async challengeHill(hillType: string, body: ChallengeHillRequest): Promise<ChallengeHillResult> {
    const res = await fetch(`${this.baseUrl}/v1/koth/hills/${hillType}/challenge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(
        `POST /v1/koth/hills/${hillType}/challenge failed: ${res.status} ${res.statusText}`,
      );
    }
    return res.json() as Promise<ChallengeHillResult>;
  }

  async submitRankedAttempt(body: RankedAttemptRequest): Promise<RankedAttemptResult> {
    const res = await fetch(`${this.baseUrl}/v1/koth/ranked/attempt`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(`POST /v1/koth/ranked/attempt failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<RankedAttemptResult>;
  }

  async getKing(hillType: string): Promise<KingInfo | null> {
    const res = await fetch(`${this.baseUrl}/v1/koth/hills/${hillType}/king`, {
      method: 'GET',
      credentials: 'include',
    });
    if (res.status === 404) {
      // Hill not seeded yet — a normal "no king yet" state, not an error.
      return null;
    }
    if (!res.ok) {
      throw new Error(`GET /v1/koth/hills/${hillType}/king failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<KingInfo>;
  }

  async getRankedLeaderboard(): Promise<RankCount[]> {
    const res = await fetch(`${this.baseUrl}/v1/koth/ranked/leaderboard`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(`GET /v1/koth/ranked/leaderboard failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<RankCount[]>;
  }

  async getRankedMe(): Promise<RankedMeResult> {
    const res = await fetch(`${this.baseUrl}/v1/koth/ranked/me`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(`GET /v1/koth/ranked/me failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<RankedMeResult>;
  }
}

export const defaultKothApi: KothApi = new KothApiClient();
