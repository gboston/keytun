// ABOUTME: End-to-end encryption using X25519 ECDH key exchange and AES-256-GCM.
// ABOUTME: Browser-side mirror of internal/crypto/crypto.go, using the Web Crypto API.

const HKDF_INFO = new TextEncoder().encode('keytun-e2e-v1');
const NONCE_SIZE = 12;

/**
 * Generate an ephemeral X25519 key pair.
 * Returns { privateKey: CryptoKey, publicKey: Uint8Array (32 bytes) }.
 */
export async function generateKeyPair() {
  const keyPair = await crypto.subtle.generateKey('X25519', false, ['deriveBits']);
  const publicKeyRaw = await crypto.subtle.exportKey('raw', keyPair.publicKey);
  return {
    privateKey: keyPair.privateKey,
    publicKey: new Uint8Array(publicKeyRaw),
  };
}

/**
 * Perform ECDH with the peer's public key and derive an AES-256-GCM key
 * via HKDF-SHA256 (info: "keytun-e2e-v1", empty salt).
 */
export async function deriveSharedKey(privateKey, peerPublicKeyBytes) {
  const peerPublicKey = await crypto.subtle.importKey(
    'raw',
    peerPublicKeyBytes,
    'X25519',
    false,
    [],
  );

  const sharedSecret = await crypto.subtle.deriveBits(
    { name: 'X25519', public: peerPublicKey },
    privateKey,
    256,
  );

  // Import the shared secret as HKDF key material
  const hkdfKey = await crypto.subtle.importKey(
    'raw',
    sharedSecret,
    'HKDF',
    false,
    ['deriveKey'],
  );

  // Derive AES-256-GCM key via HKDF-SHA256
  return crypto.subtle.deriveKey(
    {
      name: 'HKDF',
      hash: 'SHA-256',
      salt: new Uint8Array(0),
      info: HKDF_INFO,
    },
    hkdfKey,
    { name: 'AES-GCM', length: 256 },
    true, // extractable (needed for test comparison)
    ['encrypt', 'decrypt'],
  );
}

/**
 * Encrypt plaintext with AES-256-GCM.
 * Returns Uint8Array of nonce (12 bytes) || ciphertext.
 */
export async function encrypt(aesKey, plaintext) {
  const nonce = crypto.getRandomValues(new Uint8Array(NONCE_SIZE));
  const ciphertext = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: nonce },
    aesKey,
    plaintext,
  );
  const result = new Uint8Array(NONCE_SIZE + ciphertext.byteLength);
  result.set(nonce, 0);
  result.set(new Uint8Array(ciphertext), NONCE_SIZE);
  return result;
}

/**
 * Decrypt ciphertext produced by encrypt(). Expects nonce || ciphertext.
 */
export async function decrypt(aesKey, data) {
  if (data.length < NONCE_SIZE + 1) {
    throw new Error('crypto: ciphertext too short');
  }
  const nonce = data.slice(0, NONCE_SIZE);
  const ciphertext = data.slice(NONCE_SIZE);
  const plaintext = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: nonce },
    aesKey,
    ciphertext,
  );
  return new Uint8Array(plaintext);
}
