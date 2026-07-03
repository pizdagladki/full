const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface CurrentKingClip {
  download_url: string;
  blink_ts_ms: number;
}

export interface KingClipUploadResult {
  id: number;
}

export interface KingClipsApi {
  getCurrent(hillType: string): Promise<CurrentKingClip | null>;
  upload(hillType: string, blinkTsMs: number, blob: Blob): Promise<KingClipUploadResult>;
}

export class KingClipsApiClient implements KingClipsApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async getCurrent(hillType: string): Promise<CurrentKingClip | null> {
    const res = await fetch(
      `${this.baseUrl}/v1/king-clips/current?hill_type=${encodeURIComponent(hillType)}`,
      {
        method: 'GET',
        credentials: 'include',
      },
    );
    if (res.status === 404) {
      // No current clip for this hill_type yet — a normal "no opponent" state, not an error.
      return null;
    }
    if (!res.ok) {
      throw new Error(
        `GET /v1/king-clips/current failed: ${res.status} ${res.statusText}`,
      );
    }
    return res.json() as Promise<CurrentKingClip>;
  }

  async upload(hillType: string, blinkTsMs: number, blob: Blob): Promise<KingClipUploadResult> {
    const params = new URLSearchParams({
      hill_type: hillType,
      blink_ts_ms: String(blinkTsMs),
    });
    const res = await fetch(`${this.baseUrl}/v1/king-clips?${params.toString()}`, {
      method: 'POST',
      headers: { 'Content-Type': 'video/webm' },
      body: blob,
      credentials: 'include',
    });
    if (!res.ok) {
      throw new Error(`POST /v1/king-clips failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<KingClipUploadResult>;
  }
}

export const defaultKingClipsApi: KingClipsApi = new KingClipsApiClient();
