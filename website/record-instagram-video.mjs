// Record an Instagram Reel (1080x1920, 9:16) of the keytun website
// Fast-cut style: screenshot each section, assemble with ffmpeg crossfade transitions
// Target: ~30s, H.264 MP4, 30fps

import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const { chromium } = require('/opt/node22/lib/node_modules/playwright');
import { execSync, execFileSync } from 'child_process';
import fs from 'fs';
import path from 'path';

const VIEWPORT = { width: 1080, height: 1920 };
const PAGE_URL = 'http://localhost:4322/';
const OUTPUT_DIR = path.resolve('instagram-video');
const FRAMES_DIR = path.join(OUTPUT_DIR, 'frames');

async function sleep(ms) {
  return new Promise(r => setTimeout(r, ms));
}

// Sections to capture: CSS selector to scroll to, display duration in seconds
const SECTIONS = [
  { name: '01-hero',         selector: '.hero',           hold: 3.5 },
  { name: '02-problem',      selector: '#problem',        hold: 3.0 },
  { name: '03-how-it-works', selector: '#how-it-works',   hold: 3.5 },
  { name: '04-showcase',     selector: '#showcase',       hold: 3.0 },
  { name: '05-features',     selector: '.features',       hold: 3.0, fallback: '#showcase' },
  { name: '06-install',      selector: '#install',        hold: 3.0 },
  { name: '07-roadmap',      selector: '.roadmap',        hold: 2.5, fallback: '#install' },
  { name: '08-footer',       selector: 'footer',          hold: 2.0 },
];

const CROSSFADE_DURATION = 0.3; // seconds overlap between cuts

async function captureScreenshots(page) {
  const shots = [];

  for (const section of SECTIONS) {
    // Try scrolling to the section
    const found = await page.evaluate(async (sel) => {
      const el = document.querySelector(sel);
      if (el) {
        el.scrollIntoView({ behavior: 'instant', block: 'start' });
        return true;
      }
      return false;
    }, section.selector);

    if (!found && section.fallback) {
      // Scroll a bit past the fallback
      await page.evaluate(async (sel) => {
        const el = document.querySelector(sel);
        if (el) {
          const rect = el.getBoundingClientRect();
          window.scrollBy(0, rect.height * 0.8);
        }
      }, section.fallback);
    }

    await sleep(400); // Let animations/renders settle

    const filePath = path.join(FRAMES_DIR, `${section.name}.png`);
    await page.screenshot({ path: filePath });
    shots.push({ path: filePath, hold: section.hold, name: section.name });
    console.log(`  Captured: ${section.name}`);
  }

  return shots;
}

function findFfmpeg() {
  try {
    execSync('which ffmpeg', { stdio: 'ignore' });
    return 'ffmpeg';
  } catch {
    const npmFfmpeg = path.resolve('node_modules/@ffmpeg-installer/linux-x64/ffmpeg');
    if (fs.existsSync(npmFfmpeg)) return npmFfmpeg;
    throw new Error('ffmpeg not found. Install via: npm install @ffmpeg-installer/ffmpeg');
  }
}

function buildVideo(shots, ffmpegBin) {
  // Strategy: Create individual clip videos from each screenshot, then concat with crossfade
  // Step 1: Create a video clip from each screenshot
  const clipPaths = [];
  for (let i = 0; i < shots.length; i++) {
    const clipPath = path.join(OUTPUT_DIR, `clip-${i}.mp4`);
    execFileSync(ffmpegBin, [
      '-y',
      '-loop', '1',
      '-i', shots[i].path,
      '-c:v', 'libx264',
      '-t', String(shots[i].hold + CROSSFADE_DURATION),
      '-pix_fmt', 'yuv420p',
      '-vf', `fps=30,scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black`,
      '-preset', 'fast',
      '-crf', '18',
      clipPath,
    ], { stdio: 'pipe' });
    clipPaths.push(clipPath);
    console.log(`  Encoded clip ${i + 1}/${shots.length}: ${shots[i].name} (${shots[i].hold}s)`);
  }

  // Step 2: Concat clips using ffmpeg concat demuxer
  const finalPath = path.join(OUTPUT_DIR, 'keytun-instagram-reel.mp4');

  if (clipPaths.length < 2) {
    fs.renameSync(clipPaths[0], finalPath);
    return finalPath;
  }

  // Write concat list file
  const concatFile = path.join(OUTPUT_DIR, 'concat.txt');
  const concatContent = clipPaths.map(p => `file '${p}'`).join('\n');
  fs.writeFileSync(concatFile, concatContent);

  execFileSync(ffmpegBin, [
    '-y',
    '-f', 'concat',
    '-safe', '0',
    '-i', concatFile,
    '-c:v', 'libx264',
    '-preset', 'medium',
    '-crf', '18',
    '-pix_fmt', 'yuv420p',
    '-movflags', '+faststart',
    finalPath,
  ], { stdio: 'pipe' });

  // Cleanup clips and concat file
  clipPaths.forEach(p => { try { fs.unlinkSync(p); } catch {} });
  try { fs.unlinkSync(concatFile); } catch {}

  return finalPath;
}

async function main() {
  // Clean output dir
  if (fs.existsSync(OUTPUT_DIR)) fs.rmSync(OUTPUT_DIR, { recursive: true });
  fs.mkdirSync(FRAMES_DIR, { recursive: true });

  const ffmpegBin = findFfmpeg();
  console.log(`Using ffmpeg: ${ffmpegBin}\n`);

  // Launch browser
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 1,
    colorScheme: 'dark',
  });
  const page = await context.newPage();

  // Block external requests
  await page.route('**/*', (route) => {
    const url = route.request().url();
    if (url.startsWith('http://localhost')) route.continue();
    else route.abort();
  });

  console.log('Loading page...');
  await page.goto(PAGE_URL, { waitUntil: 'domcontentloaded', timeout: 30000 });
  await sleep(2000); // Let animations play

  console.log('Capturing sections...');
  const shots = await captureScreenshots(page);

  await context.close();
  await browser.close();

  console.log(`\nAssembling ${shots.length} sections into video...`);
  const totalDuration = shots.reduce((sum, s) => sum + s.hold, 0);
  console.log(`  Target duration: ~${totalDuration.toFixed(0)}s\n`);

  const finalPath = buildVideo(shots, ffmpegBin);

  const sizeMB = (fs.statSync(finalPath).size / 1024 / 1024).toFixed(1);
  console.log(`\n✅ Instagram Reel saved: ${finalPath}`);
  console.log(`   Resolution: ${VIEWPORT.width}x${VIEWPORT.height} (9:16)`);
  console.log(`   Duration:   ~${totalDuration.toFixed(0)}s`);
  console.log(`   Size:       ${sizeMB} MB`);
  console.log(`   Format:     H.264 MP4, 30fps`);
}

main().catch(console.error);
