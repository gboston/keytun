// ABOUTME: Vitest configuration for browser-compatible crypto and client tests.
// ABOUTME: Uses Node environment with Web Crypto API available via globalThis.crypto.
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['src/**/*.test.js'],
  },
});
