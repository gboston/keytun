// ABOUTME: WebSocket client for the keytun relay protocol.
// ABOUTME: Handles connection, key exchange, encrypted message send/receive.
import { generateKeyPair, deriveSharedKey, encrypt, decrypt } from './keytun-crypto.js';

const HANDSHAKE_TIMEOUT_MS = 5000;

/**
 * Convert Uint8Array to standard base64 string.
 */
export function uint8ToBase64(bytes) {
  let binary = '';
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

/**
 * Convert standard base64 string to Uint8Array.
 */
export function base64ToUint8(b64) {
  const binary = atob(b64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

/**
 * Detects double-Escape keypress to trigger disconnect.
 * Mirrors internal/client/escape.go behavior.
 */
export class EscapeDetector {
  constructor(timeoutMs = 300) {
    this.timeoutMs = timeoutMs;
    this.pending = false;
    this.escapeTime = 0;
    this.flushedEscape = false;
  }

  /**
   * Feed a byte value. Returns:
   * - 'pass': normal byte, send it
   * - 'held': escape held pending, don't send yet
   * - 'flush': non-escape after pending escape; caller should send escape + this byte
   * - 'disconnect': double-escape detected
   */
  feed(byte) {
    this.flushedEscape = false;

    if (byte === 0x1b) {
      if (this.pending && (Date.now() - this.escapeTime) < this.timeoutMs) {
        this.pending = false;
        return 'disconnect';
      }
      this.pending = true;
      this.escapeTime = Date.now();
      return 'held';
    }

    if (this.pending) {
      this.pending = false;
      this.flushedEscape = true;
      return 'flush';
    }

    return 'pass';
  }
}

/**
 * KeytunClient handles the full WebSocket protocol for joining a session.
 */
export class KeytunClient {
  constructor(relayURL, sessionCode) {
    this.relayURL = relayURL;
    this.sessionCode = sessionCode;
    this._ws = null;
    this._aesKey = null;
    this._messageWaiters = [];

    // Callbacks (set by consumer)
    this.onOutput = null;
    this.onPeerEvent = null;
    this.onError = null;
    this.onClose = null;
  }

  /**
   * Connect to the relay and complete the key exchange.
   * Resolves when the encrypted session is ready.
   */
  async connect() {
    await this._openWebSocket();
    this._setupMessageHandler();

    // Send client_join and wait for session_joined
    this._sendJoin();
    const joinAck = await this._waitForMessage('session_joined', HANDSHAKE_TIMEOUT_MS);
    if (!joinAck) {
      throw new Error('Timed out waiting for session confirmation');
    }

    // Generate key pair and exchange
    const keyPair = await generateKeyPair();
    this._sendKeyExchange(keyPair.publicKey);

    const kxMsg = await this._waitForMessage('key_exchange', HANDSHAKE_TIMEOUT_MS);
    if (!kxMsg) {
      throw new Error('Timed out waiting for key exchange');
    }

    // Derive shared AES key
    const peerPubBytes = base64ToUint8(kxMsg.data);
    this._aesKey = await deriveSharedKey(keyPair.privateKey, peerPubBytes);
  }

  /**
   * Encrypt and send input bytes to the host.
   */
  async sendInput(bytes) {
    if (!this._aesKey) throw new Error('Not connected');
    const encrypted = await encrypt(this._aesKey, bytes);
    this._sendRawInput(encrypted);
  }

  /**
   * Close the WebSocket connection.
   */
  disconnect() {
    if (this._ws) {
      this._ws.close();
    }
  }

  // --- Internal methods (exposed for testing) ---

  _sendJoin() {
    this._ws.send(JSON.stringify({
      type: 'client_join',
      session: this.sessionCode,
    }));
  }

  _sendKeyExchange(publicKey) {
    this._ws.send(JSON.stringify({
      type: 'key_exchange',
      data: uint8ToBase64(publicKey),
    }));
  }

  _sendRawInput(encryptedData) {
    this._ws.send(JSON.stringify({
      type: 'input',
      data: uint8ToBase64(encryptedData),
    }));
  }

  _setupMessageHandler() {
    this._ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);

      // Check if any waiters match this message type
      for (let i = this._messageWaiters.length - 1; i >= 0; i--) {
        const waiter = this._messageWaiters[i];
        if (waiter.type === msg.type) {
          this._messageWaiters.splice(i, 1);
          waiter.resolve(msg);
          return;
        }
      }

      // Route to callbacks
      switch (msg.type) {
        case 'output':
          if (this.onOutput && this._aesKey) {
            const ciphertext = base64ToUint8(msg.data);
            decrypt(this._aesKey, ciphertext)
              .then((plaintext) => this.onOutput(plaintext))
              .catch(() => {}); // ignore decrypt errors for corrupted frames
          } else if (this.onOutput) {
            this.onOutput(msg.data);
          }
          break;
        case 'peer_event':
          if (this.onPeerEvent) this.onPeerEvent(msg.event);
          break;
        case 'error':
          if (this.onError) this.onError(msg.message);
          break;
      }
    };

    this._ws.onclose = () => {
      if (this.onClose) this.onClose();
    };
  }

  _waitForMessage(type, timeoutMs) {
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        const idx = this._messageWaiters.findIndex((w) => w.type === type);
        if (idx >= 0) this._messageWaiters.splice(idx, 1);
        resolve(null);
      }, timeoutMs);

      this._messageWaiters.push({
        type,
        resolve: (msg) => {
          clearTimeout(timer);
          resolve(msg);
        },
      });
    });
  }

  _openWebSocket() {
    return new Promise((resolve, reject) => {
      this._ws = new WebSocket(this.relayURL);
      this._ws.onopen = () => resolve();
      this._ws.onerror = (err) => reject(new Error('WebSocket connection failed'));
    });
  }
}
