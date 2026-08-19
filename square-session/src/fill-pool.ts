import { Firestore } from '@google-cloud/firestore';
import https from 'https';
import { URL } from 'url';
import { randomUUID } from 'crypto';
import { loadSession } from './session';

// Square コードの有効期限は実質無期限（20年先）にし、有効期限はサーバー側で管理する
const SQUARE_EXPIRES_AT = '2046-01-01T00:00:00.000Z';
const POOL_COLLECTION = 'square_pool';

function requireEnv(name: string): string {
  const val = process.env[name];
  if (!val) throw new Error(`環境変数 ${name} が未設定です`);
  return val;
}

// _sqweb_session の値から _csrf_token を取り出す（Go の extractCSRF と同ロジック）
function extractCSRF(webSession: string): string {
  const decoded = decodeURIComponent(webSession);
  const parts = decoded.split('--');
  const raw = Buffer.from(parts[0], 'base64').toString('utf8');
  const payload = JSON.parse(raw) as { _csrf_token?: string };
  const csrf = payload['_csrf_token'];
  if (!csrf) throw new Error('_csrf_token が見つかりません');
  return csrf;
}

function generateMemberCode(): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  let code = 'MBR';
  for (let i = 0; i < 8; i++) {
    code += chars[Math.floor(Math.random() * chars.length)];
  }
  return code;
}

function httpsRequest<T>(
  url: string,
  method: 'GET' | 'POST',
  headers: Record<string, string>,
  body?: unknown,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const bodyStr = body !== undefined ? JSON.stringify(body) : undefined;
    const req = https.request(
      {
        hostname: parsed.hostname,
        path: parsed.pathname + parsed.search,
        method,
        headers: {
          'Content-Type': 'application/json',
          Accept: 'application/json',
          ...(bodyStr ? { 'Content-Length': String(Buffer.byteLength(bodyStr)) } : {}),
          ...headers,
        },
      },
      (res) => {
        const chunks: Buffer[] = [];
        res.on('data', (chunk: Buffer) => chunks.push(chunk));
        res.on('end', () => {
          const text = Buffer.concat(chunks).toString('utf8');
          if (res.statusCode && res.statusCode >= 400) {
            reject(new Error(`HTTP ${res.statusCode}: ${text}`));
            return;
          }
          try {
            resolve(JSON.parse(text) as T);
          } catch {
            reject(new Error(`JSON parse error: ${text}`));
          }
        });
      },
    );
    req.on('error', reject);
    if (bodyStr) req.write(bodyStr);
    req.end();
  });
}

async function notifyDiscord(webhookUrl: string, message: string): Promise<void> {
  const url = new URL(webhookUrl);
  const body = JSON.stringify({ content: message });
  return new Promise((resolve, reject) => {
    const req = https.request(
      {
        hostname: url.hostname,
        path: url.pathname,
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Content-Length': String(Buffer.byteLength(body)),
        },
      },
      (res) => {
        res.resume();
        res.on('end', resolve);
      },
    );
    req.on('error', reject);
    req.write(body);
    req.end();
  });
}

async function getPricingRuleVersion(apiToken: string, pricingRuleId: string): Promise<number> {
  const result = await httpsRequest<{
    object?: { version?: number };
    errors?: { code: string; detail: string }[];
  }>(`https://connect.squareup.com/v2/catalog/object/${pricingRuleId}`, 'GET', {
    Authorization: `Bearer ${apiToken}`,
    'Square-Version': '2025-01-23',
  });
  if (result.errors && result.errors.length > 0) {
    throw new Error(`${result.errors[0].code}: ${result.errors[0].detail}`);
  }
  if (!result.object?.version) {
    throw new Error(`pricing rule ${pricingRuleId} が見つかりません`);
  }
  return result.object.version;
}

async function createDiscountCode(
  webSession: string,
  csrfToken: string,
  code: string,
  pricingRuleId: string,
  version: number,
): Promise<{ id: string; code: string }> {
  const result = await httpsRequest<{
    discount_code?: { id: string; code: string };
    errors?: { code: string; detail: string }[];
  }>(
    'https://connect.squareup.com/v2/discount-codes',
    'POST',
    {
      'x-csrf-token': csrfToken,
      Cookie: `_sqweb_session=${webSession}`,
      Origin: 'https://app.squareup.com',
      Referer: 'https://app.squareup.com/dashboard/customers/marketing/coupons/new',
      'User-Agent':
        'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36',
      'x-allow-cookies': ',C0001,C0002,C0003,C0004,',
      'x-block-cookies': 'true',
    },
    {
      idempotency_key: randomUUID(),
      discount_code: {
        code,
        pricing_rule_id: pricingRuleId,
        max_redemptions: 1,
        expires_at: SQUARE_EXPIRES_AT,
        reason: 'COUPON_BUILDER',
        pricing_rule_version: version,
        max_redemptions_per_customer: 1,
      },
    },
  );
  if (result.errors && result.errors.length > 0) {
    throw new Error(`${result.errors[0].code}: ${result.errors[0].detail}`);
  }
  if (!result.discount_code) {
    throw new Error('discount_code が応答に含まれていません');
  }
  return result.discount_code;
}

async function fetchRulesFromCatalog(db: Firestore): Promise<{ id: string; name: string }[]> {
  const snapshot = await db.collection('reward_catalog').where('active', '==', true).get();
  const seen = new Map<string, string>(); // pricing_rule_id -> title
  for (const doc of snapshot.docs) {
    const data = doc.data() as { pricing_rule_id?: string; title?: string };
    if (data.pricing_rule_id) {
      seen.set(data.pricing_rule_id, data.title ?? data.pricing_rule_id);
    }
  }
  return Array.from(seen.entries()).map(([id, name]) => ({ id, name }));
}

async function run(): Promise<void> {
  const bucketName = requireEnv('GCS_BUCKET');
  const projectId = requireEnv('PROJECT_ID');
  const apiToken = requireEnv('SQUARE_API_TOKEN');
  const countPerRule = parseInt(process.env.FILL_COUNT ?? '50', 10);
  const delayMs = parseInt(process.env.FILL_DELAY_MS ?? '300', 10);
  const webhookUrl = process.env.DISCORD_WEBHOOK_URL;

  const db = new Firestore({ projectId });

  // reward_catalog から pricing_rule_id を取得
  const rules = await fetchRulesFromCatalog(db);
  if (rules.length === 0) {
    throw new Error('pricing_rule_id が設定されたアクティブな特典が reward_catalog に見つかりません');
  }
  console.log(`対象ルール: ${rules.map((r) => `${r.name} (${r.id})`).join(', ')}`);

  const session = await loadSession(bucketName);
  if (!session) {
    throw new Error(
      'GCS にセッションが見つかりません。先に npm run login:manual を実行してください',
    );
  }

  const sqwebCookie = session.cookies.find((c) => c.name === '_sqweb_session');
  if (!sqwebCookie) throw new Error('_sqweb_session cookie が見つかりません');

  const webSession = sqwebCookie.value;
  const csrfToken = extractCSRF(webSession);

  const summaryLines: string[] = [];

  for (const rule of rules) {
    console.log(`\n[${rule.name}] ${countPerRule} 件作成開始...`);
    let created = 0;
    let failed = 0;

    const version = await getPricingRuleVersion(apiToken, rule.id);
    console.log(`  pricing rule version: ${version}`);

    for (let i = 0; i < countPerRule; i++) {
      const code = generateMemberCode();
      try {
        const dc = await createDiscountCode(webSession, csrfToken, code, rule.id, version);
        await db.collection(POOL_COLLECTION).add({
          pricing_rule_id: rule.id,
          code: dc.code,
          discount_code_id: dc.id,
          used: false,
          used_by: null,
          used_at: null,
          created_at: new Date(),
        });
        created++;
        process.stdout.write('.');
      } catch (err) {
        failed++;
        console.error(`\n  失敗 (${code}):`, err instanceof Error ? err.message : err);
      }

      if (i < countPerRule - 1) {
        await new Promise((r) => setTimeout(r, delayMs));
      }
    }

    const line = `[${rule.name}] 作成: ${created} 件 / 失敗: ${failed} 件`;
    console.log(`\n  ${line}`);
    summaryLines.push(line);
  }

  console.log('\n✅ 完了');

  if (webhookUrl) {
    const msg = `✅ Square クーポンプール補充完了\n${summaryLines.map((l) => `• ${l}`).join('\n')}`;
    await notifyDiscord(webhookUrl, msg);
  }
}

run().catch((err) => {
  console.error('エラー:', err instanceof Error ? err.message : err);
  process.exit(1);
});
