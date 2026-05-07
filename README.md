# members-site

FOOD RECORDS メンバーズの LIFF フロントエンド。
GitHub Pages でホスティングされ、LINE アプリ内から開く。

- 本番 URL: https://members.food-records.com
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

## デプロイ

```bash
git add config.js index.html liff.js
git commit -m "変更内容"
git push origin master
```

`master` ブランチへの push で GitHub Pages に自動反映される（反映まで約1〜2分）。

## 環境設定の切り替え

| 環境 | config.js の apiUrl |
|---|---|
| 本番 | `https://members-api-438247672947.us-central1.run.app` |
| ローカル開発 | ngrok 等で公開したローカル API の URL |
