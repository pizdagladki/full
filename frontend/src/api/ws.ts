const DEFAULT_WS_URL = (import.meta.env?.VITE_WS_URL as string | undefined) ?? '';

// Injectable WS client contract — production wraps the native WebSocket (WsClient); tests provide
// a plain mock object implementing the same surface.
export interface WsClientApi {
  connect(path: string): void;
  send(data: string): void;
  close(): void;
  onMessage(cb: (data: string) => void): void;
  onOpen(cb: () => void): void;
  onClose(cb: () => void): void;
}

export class WsClient implements WsClientApi {
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

  onOpen(cb: () => void): void {
    if (!this.socket) {
      throw new Error('WsClient: not connected');
    }
    this.socket.onopen = () => {
      cb();
    };
  }

  onClose(cb: () => void): void {
    if (!this.socket) {
      throw new Error('WsClient: not connected');
    }
    this.socket.onclose = () => {
      cb();
    };
  }
}
