const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface RatingData {
  elo: number;
  level: number;
  games_played: number;
}

export interface RatingsApi {
  getRating(userId: string): Promise<RatingData>;
}

export class RatingsApiClient implements RatingsApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async getRating(userId: string): Promise<RatingData> {
    const res = await fetch(`${this.baseUrl}/v1/ratings/${userId}`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(`GET /v1/ratings/${userId} failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<RatingData>;
  }
}

export const defaultRatingsApi: RatingsApi = new RatingsApiClient();
