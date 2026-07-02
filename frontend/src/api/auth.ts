const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface ConsentInfo {
  is_adult: boolean;
  consent_recording: boolean;
  consent_tos: boolean;
  accepted_at: string;
}

export interface ConsentPayload {
  is_adult: boolean;
  consent_recording: boolean;
  consent_tos: boolean;
}

export interface User {
  id: string;
  email: string;
  consent?: ConsentInfo | null;
}

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export interface AuthApi {
  googleLogin(code: string): Promise<void>;
  getMe(): Promise<User>;
  submitConsent(payload: ConsentPayload): Promise<ConsentInfo>;
}

export class AuthApiClient implements AuthApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async googleLogin(code: string): Promise<void> {
    const res = await fetch(`${this.baseUrl}/v1/auth/google`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code }),
      credentials: 'include',
    });
    if (!res.ok) {
      throw new ApiError(res.status, `POST /v1/auth/google failed: ${res.status} ${res.statusText}`);
    }
  }

  async getMe(): Promise<User> {
    const res = await fetch(`${this.baseUrl}/v1/auth/me`, {
      method: 'GET',
      credentials: 'include',
    });
    if (!res.ok) {
      throw new ApiError(res.status, `GET /v1/auth/me failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<User>;
  }

  async submitConsent(payload: ConsentPayload): Promise<ConsentInfo> {
    const res = await fetch(`${this.baseUrl}/v1/auth/consent`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      credentials: 'include',
    });
    if (!res.ok) {
      throw new ApiError(res.status, `POST /v1/auth/consent failed: ${res.status} ${res.statusText}`);
    }
    return res.json() as Promise<ConsentInfo>;
  }
}

export const defaultAuthApi: AuthApi = new AuthApiClient();
