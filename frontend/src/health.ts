export interface Health {
  status: string;
}

/** healthStatus mirrors the backend GET /v1/health contract. */
export function healthStatus(): Health {
  return { status: 'ok' };
}
