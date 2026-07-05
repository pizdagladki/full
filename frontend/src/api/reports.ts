const DEFAULT_BASE_URL = (import.meta.env?.VITE_API_URL as string | undefined) ?? '';

export interface CheatReportRequest {
  reported_id: number;
  match_id: string;
}

export type BugReportDevice = 'mobile' | 'pc';

export interface BugReportRequest {
  device: BugReportDevice;
  description?: string;
  /** PC path only — the full recording MAY be attached; mobile is text-only. */
  recording?: Blob;
}

export interface ReportsApi {
  reportCheat(payload: CheatReportRequest): Promise<void>;
  reportBug(payload: BugReportRequest): Promise<void>;
}

export class ReportsApiClient implements ReportsApi {
  private readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_BASE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  async reportCheat(payload: CheatReportRequest): Promise<void> {
    const res = await fetch(`${this.baseUrl}/v1/reports/cheat`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      throw new Error(`POST /v1/reports/cheat failed: ${res.status} ${res.statusText}`);
    }
  }

  async reportBug(payload: BugReportRequest): Promise<void> {
    // PC MAY attach the full recording — sent as multipart/form-data. Mobile stays text-only JSON.
    const res = payload.recording
      ? await fetch(`${this.baseUrl}/v1/reports/bug`, {
          method: 'POST',
          credentials: 'include',
          body: toFormData(payload),
        })
      : await fetch(`${this.baseUrl}/v1/reports/bug`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ device: payload.device, description: payload.description }),
        });
    if (!res.ok) {
      throw new Error(`POST /v1/reports/bug failed: ${res.status} ${res.statusText}`);
    }
  }
}

function toFormData(payload: BugReportRequest): FormData {
  const form = new FormData();
  form.append('device', payload.device);
  if (payload.description) form.append('description', payload.description);
  if (payload.recording) form.append('recording', payload.recording);
  return form;
}

export const defaultReportsApi: ReportsApi = new ReportsApiClient();
