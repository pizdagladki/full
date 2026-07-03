const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface PointsBalance {
  balance: number;
}

export interface PointsApi {
  getBalance(): Promise<PointsBalance>;
}

export class PointsApiClient implements PointsApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async getBalance(): Promise<PointsBalance> {
    const res = await fetch(`${this.baseUrl}/v1/points/balance`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(`GET /v1/points/balance failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<PointsBalance>;
  }
}

export const defaultPointsApi: PointsApi = new PointsApiClient();
