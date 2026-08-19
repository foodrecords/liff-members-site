import { chromium } from 'playwright-extra';
import StealthPlugin from 'puppeteer-extra-plugin-stealth';
import { Storage } from '@google-cloud/storage';
import https from 'https';
import { saveSession, type StorageState } from './session';

chromium.use(StealthPlugin());

const LOGIN_URL = 'https://app.squareup.com/login';

function requireEnv(name: string): string {
  const val = process.env[name];
  if (!val) throw new Error(`環境変数 ${name} が未設定です`);
  return val;
}

async function notifyDiscord(message: string): Promise<void> {
  const webhookUrl = process.env.DISCORD_WEBHOOK_URL;
  if (!webhookUrl) {
    console.log('[Discord通知スキップ] DISCORD_WEBHOOK_URL 未設定');
    return;
  }
  const url = new URL(webhookUrl);
  const body = JSON.stringify({ content: message });
  return new Promise((resolve, reject) => {
    const req = https.request(
      { hostname: url.hostname, path: url.pathname, method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) } },
      (res) => { res.resume(); res.on('end', resolve); }
    );
    req.on('error', reject);
    req.write(body);
    req.end();
  });
}

async function run(): Promise<void> {
  const email = requireEnv('SQUARE_EMAIL');
  const password = requireEnv('SQUARE_PASSWORD');
  const bucketName = requireEnv('GCS_BUCKET');

  const browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage'],
  });

  const context = await browser.newContext({
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36',
    locale: 'ja-JP',
    timezoneId: 'Asia/Tokyo',
    viewport: { width: 1280, height: 800 },
  });

  const page = await context.newPage();

  try {
    console.log('ログインページを開きます...');
    await page.goto(LOGIN_URL, { waitUntil: 'networkidle', timeout: 30000 });

    await page.fill('input[name="email"], input[type="email"]', email);

    const nextButton = page.locator('button:has-text("次へ"), button:has-text("Next"), button:has-text("Continue")').first();
    if (await nextButton.isVisible({ timeout: 3000 }).catch(() => false)) {
      await nextButton.click();
      await page.waitForTimeout(1000);
    }

    await page.fill('input[name="password"], input[type="password"]', password);
    await page.click('button[type="submit"]');

    console.log('ログイン処理中...');
    const deadline = Date.now() + 30000;
    while (Date.now() < deadline) {
      if (!page.url().includes('/login')) break;
      await new Promise((r) => setTimeout(r, 1000));
    }
    if (page.url().includes('/login')) throw new Error('ログイン後のページ遷移がタイムアウトしました');

    console.log('ログイン成功');
    const storageState = await context.storageState() as StorageState;
    await saveSession(bucketName, storageState);

    await notifyDiscord('✅ Square セッション更新完了\n次回更新: 翌朝4時');
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    console.error('ログイン失敗:', message);

    try {
      const shot = await page.screenshot({ fullPage: true });
      const storage = new Storage();
      await storage.bucket(bucketName).file('square-login-error.png').save(shot, { contentType: 'image/png' });
    } catch (_) {}

    await notifyDiscord(
      `⚠️ Square ログイン失敗（Cloudflare ブロックの可能性）\n\`${message}\`\n\n手動ログインが必要です。営業開始前に以下を実行してください。\n\`\`\`\ncd square-session && npm run login:manual\n\`\`\``
    );
    throw err;
  } finally {
    await browser.close();
  }
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
