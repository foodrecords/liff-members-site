import { chromium } from 'playwright-extra';
import StealthPlugin from 'puppeteer-extra-plugin-stealth';
import type { BrowserContextOptions } from 'playwright';
import { saveSession, loadSession, type StorageState } from './session';

chromium.use(StealthPlugin());

const LOGIN_URL = 'https://app.squareup.com/login?lang_code=ja-JP';

async function run(): Promise<void> {
  const bucketName = process.env.GCS_BUCKET;
  if (!bucketName) throw new Error('GCS_BUCKET 環境変数が未設定です');

  const browser = await chromium.launch({
    headless: false,
    args: ['--no-sandbox'],
  });

  const existingSession = await loadSession(bucketName);

  const contextOptions: BrowserContextOptions = {
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36',
    locale: 'ja-JP',
    timezoneId: 'Asia/Tokyo',
    viewport: { width: 1280, height: 800 },
    ...(existingSession ? { storageState: existingSession as BrowserContextOptions['storageState'] } : {}),
  };

  const context = await browser.newContext(contextOptions);
  const page = await context.newPage();

  console.log('ブラウザを起動しました。ログインしてください...');
  await page.goto(LOGIN_URL);

  console.log('ログイン完了を待機中（ログインページから離れたら自動保存します）...');
  const deadline = Date.now() + 180000;
  while (Date.now() < deadline) {
    const url = page.url();
    if (!url.includes('/login')) {
      console.log(`遷移先URL: ${url}`);
      break;
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  if (page.url().includes('/login')) {
    throw new Error('タイムアウト: 3分以内にログインが完了しませんでした');
  }

  console.log('ログイン確認。セッションを保存します...');
  const storageState = await context.storageState() as StorageState;
  await saveSession(bucketName, storageState);

  await browser.close();
  console.log('完了。ブラウザを閉じました。');
}

run().catch((err) => {
  console.error('エラー:', err.message);
  process.exit(1);
});
