import { Storage } from '@google-cloud/storage';

const SESSION_FILE = 'square-session.json';
const WEB_SESSION_FILE = 'sqweb_session.txt';

export type StorageState = {
  cookies: { name: string; value: string; domain: string }[];
  origins: unknown[];
};

export async function saveSession(bucketName: string, storageState: StorageState): Promise<void> {
  const storage = new Storage();
  const bucket = storage.bucket(bucketName);

  await bucket.file(SESSION_FILE).save(JSON.stringify(storageState), { contentType: 'application/json' });
  console.log(`セッション全体を gs://${bucketName}/${SESSION_FILE} に保存しました`);

  const sqwebCookie = storageState.cookies.find((c) => c.name === '_sqweb_session');
  if (!sqwebCookie) {
    throw new Error('_sqweb_session が cookies に見つかりません');
  }
  await bucket.file(WEB_SESSION_FILE).save(sqwebCookie.value, { contentType: 'text/plain' });
  console.log(`✅ _sqweb_session を gs://${bucketName}/${WEB_SESSION_FILE} に保存しました`);
}

export async function loadSession(bucketName: string): Promise<StorageState | null> {
  try {
    const storage = new Storage();
    const [content] = await storage.bucket(bucketName).file(SESSION_FILE).download();
    console.log('既存セッションを GCS から読み込みました');
    return JSON.parse(content.toString()) as StorageState;
  } catch {
    return null;
  }
}
