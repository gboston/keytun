// Record an Instagram Reel (1080x1920, 9:16) of the keytun website
// Uses Playwright to scroll through the landing page and capture video

import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
import { execSync, execFileSync } from 'child_process';
import fs from 'fs';
import path from 'path';

const VIEWPORT = { width: 1080, height: 1920 };
const URL = 'http://localhost:4322/';
const OUTPUT_DIR = path.resolve('instagram-video');

async function sleep(ms) {
  return new Promise(r => setTimeout(r, ms));
}

async function main() {
  // Clean output dir
  if (fs.existsSync(OUTPUT_DIR)) fs.rmSync(OUTPUT_DIR, { recursive: true });
  fs.mkdirSync(OUTPUT_DIR, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 1,
    recordVideo: {
      dir: OUTPUT_DIR,
      size: VIEWPORT,
    },
    colorScheme: 'dark',
  });

  const page = await context.newPage();

  // Block external requests (analytics, fonts loading delays)
  await page.route('**/*', (route) => {
    const url = route.request().url();
    if (url.startsWith('http://localhost')) {
      route.continue();
    } else {
      route.abort();
    }
  });

  // Navigate
  console.log('Loading page...');
  await page.goto(URL, { waitUntil: 'domcontentloaded', timeout: 30000 });
  await sleep(3000); // Let hero animations play

  // Get total page height
  const totalHeight = await page.evaluate(() => document.body.scrollHeight);
  console.log(`Page height: ${totalHeight}px`);

  // Smooth scroll through the entire page
  const scrollStep = 5;
  const scrollInterval = 30;
  const pauseSections = [
    { name: 'Hero', y: 0, pause: 2500 },
    { name: 'Problem', y: 0.10, pause: 2000 },
    { name: 'How It Works', y: 0.22, pause: 2500 },
    { name: 'Showcase', y: 0.38, pause: 2500 },
    { name: 'Features', y: 0.52, pause: 2500 },
    { name: 'Install', y: 0.70, pause: 2000 },
    { name: 'Roadmap', y: 0.83, pause: 2000 },
  ];

  let currentScroll = 0;
  let nextPauseIdx = 1; // Skip hero (already paused above)

  while (currentScroll < totalHeight - VIEWPORT.height) {
    const progress = currentScroll / totalHeight;
    if (nextPauseIdx < pauseSections.length && progress >= pauseSections[nextPauseIdx].y) {
      const section = pauseSections[nextPauseIdx];
      console.log(`  Pausing at ${section.name} (${Math.round(progress * 100)}%)`);
      await sleep(section.pause);
      nextPauseIdx++;
    }

    currentScroll += scrollStep;
    await page.evaluate((y) => window.scrollTo({ top: y }), currentScroll);
    await sleep(scrollInterval);
  }

  // Pause at bottom
  console.log('  At bottom, pausing...');
  await sleep(2500);

  // Close to finalize video
  const videoPath = await page.video().path();
  await context.close();
  await browser.close();

  console.log(`\nRaw video saved: ${videoPath}`);
  console.log(`Size: ${(fs.statSync(videoPath).size / 1024 / 1024).toFixed(1)} MB`);

  // Use ffmpeg to convert to MP4 for Instagram
  const finalPath = path.join(OUTPUT_DIR, 'keytun-instagram-reel.mp4');
  // Try system ffmpeg, then npm-installed @ffmpeg-installer/ffmpeg
  let ffmpegBin = 'ffmpeg';
  try { execSync('which ffmpeg', { stdio: 'ignore' }); } catch {
    const npmFfmpeg = path.resolve('node_modules/@ffmpeg-installer/linux-x64/ffmpeg');
    if (fs.existsSync(npmFfmpeg)) ffmpegBin = npmFfmpeg;
  }
  try {
    execFileSync(ffmpegBin, [
      '-y', '-i', videoPath,
      '-vf', 'fps=30',
      '-c:v', 'libx264', '-preset', 'medium', '-crf', '20',
      '-pix_fmt', 'yuv420p', '-movflags', '+faststart',
      finalPath,
    ], { stdio: 'inherit' });
    if (fs.existsSync(videoPath) && videoPath !== finalPath) {
      fs.unlinkSync(videoPath);
    }
    const sizeMB = (fs.statSync(finalPath).size / 1024 / 1024).toFixed(1);
    console.log(`\n✅ Instagram Reel video saved: ${finalPath}`);
    console.log(`   Resolution: ${VIEWPORT.width}x${VIEWPORT.height} (9:16)`);
    console.log(`   Size: ${sizeMB} MB`);
  } catch (e) {
    console.log(`\nffmpeg conversion failed - raw video at: ${videoPath}`);
    console.log('Install ffmpeg or run: npm install @ffmpeg-installer/ffmpeg');
    console.log('Then: ffmpeg -i raw.webm -vf "fps=30" -c:v libx264 -crf 20 -pix_fmt yuv420p output.mp4');
  }
}

main().catch(console.error);
