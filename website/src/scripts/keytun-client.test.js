// ABOUTME: Unit tests for the keytun WebSocket client protocol handler.
// ABOUTME: Verifies message format, connection flow, and base64 encoding helpers.
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { KeytunClient, uint8ToBase64, base64ToUint8 } from './keytun-client.js';

describe('base64 helpers', () => {
  it('round-trips a Uint8Array through base64', () => {
    const original = new Uint8Array([0, 1, 127, 128, 255]);
    const encoded = uint8ToBase64(original);
    const decoded = base64ToUint8(encoded);
    expect(decoded).toEqual(original);
  });

  it('encodes empty array', () => {
    const encoded = uint8ToBase64(new Uint8Array(0));
    const decoded = base64ToUint8(encoded);
    expect(decoded).toEqual(new Uint8Array(0));
  });

  it('produces standard base64 (not URL-safe)', () => {
    // Bytes that produce + and / in standard base64
    const bytes = new Uint8Array([251, 239, 190]); // produces "+++"
    const encoded = uint8ToBase64(bytes);
    expect(encoded).not.toContain('-');
    expect(encoded).not.toContain('_');
  });
});

// Mock WebSocket for protocol testing
class MockWebSocket {
  constructor() {
    this.sent = [];
    this.readyState = 1; // OPEN
    this.onmessage = null;
    this.onclose = null;
    this.onerror = null;
  }

  send(data) {
    this.sent.push(JSON.parse(data));
  }

  close() {
    this.readyState = 3; // CLOSED
    if (this.onclose) this.onclose({ code: 1000 });
  }

  // Test helper: simulate receiving a message from the relay
  receive(msg) {
    if (this.onmessage) {
      this.onmessage({ data: JSON.stringify(msg) });
    }
  }
}

describe('KeytunClient', () => {
  describe('message format', () => {
    it('sends correctly formatted client_join message', () => {
      const ws = new MockWebSocket();
      const client = new KeytunClient('wss://test/ws', 'keen-fox-42');
      client._ws = ws;
      client._sendJoin();

      expect(ws.sent).toHaveLength(1);
      expect(ws.sent[0]).toEqual({
        type: 'client_join',
        session: 'keen-fox-42',
      });
    });

    it('sends correctly formatted key_exchange message', () => {
      const ws = new MockWebSocket();
      const client = new KeytunClient('wss://test/ws', 'keen-fox-42');
      client._ws = ws;

      const pubKey = new Uint8Array(32).fill(0xab);
      client._sendKeyExchange(pubKey);

      expect(ws.sent).toHaveLength(1);
      expect(ws.sent[0].type).toBe('key_exchange');
      // Verify it's valid base64 that decodes to 32 bytes
      const decoded = base64ToUint8(ws.sent[0].data);
      expect(decoded).toEqual(pubKey);
    });

    it('sends correctly formatted input message', () => {
      const ws = new MockWebSocket();
      const client = new KeytunClient('wss://test/ws', 'keen-fox-42');
      client._ws = ws;

      const encryptedData = new Uint8Array([1, 2, 3, 4, 5]);
      client._sendRawInput(encryptedData);

      expect(ws.sent).toHaveLength(1);
      expect(ws.sent[0].type).toBe('input');
      const decoded = base64ToUint8(ws.sent[0].data);
      expect(decoded).toEqual(encryptedData);
    });
  });

  describe('message routing', () => {
    it('calls onOutput when output message received', () => {
      const ws = new MockWebSocket();
      const client = new KeytunClient('wss://test/ws', 'keen-fox-42');
      client._ws = ws;

      const outputHandler = vi.fn();
      client.onOutput = outputHandler;

      client._setupMessageHandler();
      ws.receive({ type: 'output', data: uint8ToBase64(new Uint8Array([65])) });

      expect(outputHandler).toHaveBeenCalledTimes(1);
      // The handler receives the raw base64-encoded data (decryption happens in sendInput layer)
    });

    it('calls onPeerEvent when peer_event message received', () => {
      const ws = new MockWebSocket();
      const client = new KeytunClient('wss://test/ws', 'keen-fox-42');
      client._ws = ws;

      const peerHandler = vi.fn();
      client.onPeerEvent = peerHandler;

      client._setupMessageHandler();
      ws.receive({ type: 'peer_event', event: 'joined' });

      expect(peerHandler).toHaveBeenCalledWith('joined');
    });

    it('calls onError when error message received', () => {
      const ws = new MockWebSocket();
      const client = new KeytunClient('wss://test/ws', 'keen-fox-42');
      client._ws = ws;

      const errorHandler = vi.fn();
      client.onError = errorHandler;

      client._setupMessageHandler();
      ws.receive({ type: 'error', message: 'session not found' });

      expect(errorHandler).toHaveBeenCalledWith('session not found');
    });
  });

  describe('disconnect', () => {
    it('closes the WebSocket', () => {
      const ws = new MockWebSocket();
      const client = new KeytunClient('wss://test/ws', 'keen-fox-42');
      client._ws = ws;

      client.disconnect();
      expect(ws.readyState).toBe(3); // CLOSED
    });
  });
});

describe('EscapeDetector', () => {
  // Inline test since it's a small utility
  it('is tested via the client module', async () => {
    const { EscapeDetector } = await import('./keytun-client.js');

    const detector = new EscapeDetector(300);

    // Non-escape bytes pass through
    expect(detector.feed(0x41)).toBe('pass');
    expect(detector.feed(0x42)).toBe('pass');

    // First escape is held
    expect(detector.feed(0x1b)).toBe('held');

    // Second escape within timeout triggers disconnect
    expect(detector.feed(0x1b)).toBe('disconnect');
  });

  it('flushes pending escape when non-escape follows', async () => {
    const { EscapeDetector } = await import('./keytun-client.js');

    const detector = new EscapeDetector(300);

    expect(detector.feed(0x1b)).toBe('held');
    expect(detector.feed(0x41)).toBe('flush');
    expect(detector.flushedEscape).toBe(true);
  });

  it('resets after disconnect', async () => {
    const { EscapeDetector } = await import('./keytun-client.js');

    const detector = new EscapeDetector(300);

    detector.feed(0x1b);
    detector.feed(0x1b); // disconnect

    // After disconnect, next escape should be held again
    expect(detector.feed(0x1b)).toBe('held');
  });
});
