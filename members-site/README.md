# members-site

FOOD RECORDS メンバーズの LIFF フロントエンド。
GitHub Pages でホスティングされ、LINE アプリ内から開く。

- 本番 URL: https://members.agaruke.com
- LIFF ID: `2000938587-06YmdyJ5`

## 構成ファイル

| ファイル | 説明 |
|---|---|
| `config.js` | 環境設定（LIFF ID・API URL）。環境によってここだけ書き換える |
| `liff.js` | LIFF 初期化・QRスキャン・API 通信ロジック |
| `index.html` | 画面構成 |
| `index.css` | スタイル |

## ローカル開発

### 1. API URL を ngrok に切り替える

```js
// config.js
window.APP_CONFIG = {
  liffId: "2000938587-06YmdyJ5",
  apiUrl: "https://<ngrok-url>",  // ローカル API の ngrok URL に変更
};
```

### 2. ブラウザで開く

LIFF は LINE アプリ内でしか動作しないため、動作確認は LINE アプリから LIFF URL を開く。
URL に `?code=XXXXXXXX` を付与することで QRスキャンを省略できる。

既存の「QRコードを読み取る」は通常のポイントQRに加えて、Kioskが表示するLIFF URL形式の連携QRにも対応する。スマートフォン標準カメラから読み取った場合もLIFFを起動し、`kiosk_token`を使ってLINE会員を現在の会計へ紐づける。旧`AGARUKE_KIOSK:`形式も互換性のため読み取れる。

## デプロイ

```bash
git add config.js index.html liff.js
git commit -m "変更内容"
git push origin master
```

monorepoの`master`ブランチへ`members-site/**`の変更をpushすると、GitHub ActionsがこのディレクトリだけをGitHub Pagesへ反映する（反映まで約1〜2分）。

## 環境設定の切り替え

| 環境 | config.js の apiUrl |
|---|---|
| 本番 | `https://members-api-438247672947.us-central1.run.app` |
| ローカル開発 | ngrok 等で公開したローカル API の URL |
