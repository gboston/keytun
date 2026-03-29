// @ts-check
// ABOUTME: Astro configuration with site URL and sitemap generation.
// ABOUTME: Excludes functional pages (/s/*, /join) from the sitemap.
import { defineConfig } from 'astro/config';
import sitemap from '@astrojs/sitemap';

// https://astro.build/config
export default defineConfig({
  site: 'https://keytun.com',
  integrations: [
    sitemap({
      filter: (page) => !page.includes('/s/') && !page.includes('/join'),
    }),
  ],
});
