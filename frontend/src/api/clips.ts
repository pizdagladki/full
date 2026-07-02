const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface Clip {
  id: string;
  mp4_url?: string;
  created_at: string; // ISO 8601
}

export interface ClipsApi {
  getClips(): Promise<Clip[]>;
  getClipDownloadUrl(id: string): string;
}

export class ClipsApiClient implements ClipsApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async getClips(): Promise<Clip[]> {
    const res = await fetch(`${this.baseUrl}/v1/clips`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(`GET /v1/clips failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<Clip[]>;
  }

  getClipDownloadUrl(id: string): string {
    return `${this.baseUrl}/v1/clips/${id}/download`;
  }
}

export const defaultClipsApi: ClipsApi = new ClipsApiClient();
