const DEFAULT_WS_URL = (import.meta.env?.VITE_WS_URL as string | undefined) ?? '';

export class WsClient {
  private readonly baseUrl: string;
  private socket: WebSocket | null = null;

  constructor(baseUrl: string = DEFAULT_WS_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  connect(path: string): void {
    if (this.socket) {
      this.socket.close();
    }
    this.socket = new WebSocket(`${this.baseUrl}${path}`);
  }

  send(data: string): void {
    if (!this.socket) {
      throw new Error('WsClient: not connected');
    }
    this.socket.send(data);
  }

  close(): void {
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
  }

  onMessage(cb: (data: string) => void): void {
    if (!this.socket) {
      throw new Error('WsClient: not connected');
    }
    this.socket.onmessage = (event: MessageEvent<string>) => {
      cb(event.data);
    };
  }
}
