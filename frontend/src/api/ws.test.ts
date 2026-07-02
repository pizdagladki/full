import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { WsClient } from './ws';

// Mock WebSocket — simulates the native browser WebSocket API
class MockWebSocket {
  url: string;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  readyState = 1; // OPEN
  static instances: MockWebSocket[] = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  send = vi.fn();
  close = vi.fn();

  simulateMessage(data: string) {
    this.onmessage?.({ data } as MessageEvent);
  }

  simulateOpen() {
    this.onopen?.();
  }

  simulateClose() {
    this.onclose?.();
  }
}

describe('WsClient', () => {
  const BASE = 'ws://ws.test';

  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal('WebSocket', MockWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // criterion: ws-connect — connect creates WebSocket with the correct URL
  it('connect creates WebSocket with correct URL', () => {
    const client = new WsClient(BASE);
    client.connect('/room/1');
    expect(MockWebSocket.instances[0].url).toBe('ws://ws.test/room/1');
  });

  // criterion: ws-send — send forwards data to the underlying WebSocket
  it('send forwards data to the WebSocket', () => {
    const client = new WsClient(BASE);
    client.connect('/chat');
    client.send('hello');
    expect(MockWebSocket.instances[0].send).toHaveBeenCalledWith('hello');
  });

  // criterion: ws-onmessage — onMessage callback fires when the WebSocket receives a message
  it('onMessage callback fires on message', () => {
    const client = new WsClient(BASE);
    client.connect('/chat');
    const cb = vi.fn();
    client.onMessage(cb);
    MockWebSocket.instances[0].simulateMessage('ping');
    expect(cb).toHaveBeenCalledWith('ping');
  });

  // criterion: ws-close — close closes the underlying WebSocket
  it('close closes the WebSocket', () => {
    const client = new WsClient(BASE);
    client.connect('/chat');
    client.close();
    expect(MockWebSocket.instances[0].close).toHaveBeenCalled();
  });

  // criterion: ws-connect — fails if URL is constructed incorrectly (baseUrl + path)
  it('connect URL is baseUrl concatenated with path — fails if path is dropped', () => {
    const client = new WsClient(BASE);
    client.connect('/room/99');
    const ws = MockWebSocket.instances[0];
    // Both segments must appear in the URL — fails if only BASE or only path is used
    expect(ws.url).toContain('ws://ws.test');
    expect(ws.url).toContain('/room/99');
  });

  // criterion: ws-send — fails if send is called before connect (throws)
  it('send throws if not connected', () => {
    const client = new WsClient(BASE);
    expect(() => client.send('msg')).toThrow();
  });

  // criterion: ws-onmessage — fails if onMessage is registered before connect (throws)
  it('onMessage throws if not connected', () => {
    const client = new WsClient(BASE);
    expect(() => client.onMessage(vi.fn())).toThrow();
  });

  // criterion: ws-onopen — onOpen callback fires when the WebSocket opens
  it('onOpen callback fires when the WebSocket opens', () => {
    const client = new WsClient(BASE);
    client.connect('/chat');
    const cb = vi.fn();
    client.onOpen(cb);
    MockWebSocket.instances[0].simulateOpen();
    expect(cb).toHaveBeenCalled();
  });

  // criterion: ws-onopen — fails if onOpen is registered before connect (throws)
  it('onOpen throws if not connected', () => {
    const client = new WsClient(BASE);
    expect(() => client.onOpen(vi.fn())).toThrow();
  });

  // criterion: ws-onclose — onClose callback fires when the WebSocket closes
  it('onClose callback fires when the WebSocket closes', () => {
    const client = new WsClient(BASE);
    client.connect('/chat');
    const cb = vi.fn();
    client.onClose(cb);
    MockWebSocket.instances[0].simulateClose();
    expect(cb).toHaveBeenCalled();
  });

  // criterion: ws-onclose — fails if onClose is registered before connect (throws)
  it('onClose throws if not connected', () => {
    const client = new WsClient(BASE);
    expect(() => client.onClose(vi.fn())).toThrow();
  });
});
