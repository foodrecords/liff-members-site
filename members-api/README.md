# members-api

FOOD RECORDS メンバーズの Go API サーバー。Cloud Run で稼働する。

- 本番 URL: https://members-api-438247672947.us-central1.run.app
- ランタイム: Go 1.21
- データストア: Firestore

## API エンドポイント

### GET /members

LINE アクセストークンでユーザーを特定し、ポイント残高とメンバー番号を返す。
メンバー未登録の場合は新規作成する。

**リクエスト**
```
Authorization: Bearer <LINE アクセストークン>
```

**レスポンス**
```json
{
  "data": {
    "number": "FR000042",
    "point": 300
  }
}
```

### POST /qrcode

シリアルナンバーを照合し、ポイントを付与する。

**リクエスト**
```
Authorization: Bearer <LINE アクセストークン>

{ "code": "FRABCD1234" }
```

**レスポンス**
```json
{
  "data": {
    "number": "FR000042",
    "get_point": 100,
    "point": 400
  }
}
```

**エラー**
| ステータス | メッセージ | 原因 |
|---|---|---|
| 400 | `シリアルナンバーが見つかりません` | 存在しないコード |
| 400 | `このシリアルナンバーは既に使われています` | 使用済みコード |
| 403 | `invalid token` | 無効な LINE トークン |

## ローカル開発

### 必要なもの

- Go 1.21+
- Docker

### 起動

```bash
# 環境変数ファイルをコピー
cp etc/docker/.env.default etc/docker/.env
# etc/docker/.env の PROJECT_ID を実際の Firebase プロジェクト ID に書き換える

# Firebase 認証用のサービスアカウントキーを配置
cp <サービスアカウントJSON> key.json

# 起動
make ud
```

`.env` の設定項目:

| キー | 説明 |
|---|---|
| `ENV` | `local` を指定すると `key.json` を使って Firebase に接続する |
| `PROJECT_ID` | Firebase プロジェクト ID |
| `MEMBERS_SERVICE_KEY` | kiosk APIとの内部通信に共有する十分に長いランダム秘密値。FirebaseやLINEの既存キーではなく、新規生成する |

## セルフレジ連携API

- `POST /kiosk/checkouts/link`: LIFFがLINEアクセストークン付きでKiosk表示QRのトークンを会員へ紐づける。
- `POST /kiosk/checkout-tokens`: kiosk APIが2分有効の連携トークンを発行する。
- `POST /kiosk/checkouts/resolve`: kiosk APIが連携完了を確認し、会員・利用可能クーポンを取得する。
- クーポン予約・取消・注文確定APIを含む内部APIは`X-Members-Service-Key`で認証する。

### ローカル API を外部公開（LIFF からのアクセス用）

LIFF はブラウザから動作するため、`localhost` には直接アクセスできない。ngrok 等で公開する。

```bash
ngrok http 8080
```

発行された URL を `members-site/config.js` の `apiUrl` に設定する。

## デプロイ（Cloud Run）

```bash
# イメージをビルドして push
docker build -f etc/docker/api/Dockerfile.prod -t gcr.io/food-records-prod/members-api .
docker push gcr.io/food-records-prod/members-api

# Cloud Run にデプロイ
gcloud run deploy members-api \
  --platform managed \
  --image gcr.io/food-records-prod/members-api \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars PROJECT_ID=fr-agaruke
```

## パッケージ構成

```
members-api/
├── cmd/api/
│   ├── main.go                   エントリポイント・ルーター定義
│   └── adapter/
│       ├── healthcheck/          GET /
│       ├── member/               GET /members
│       └── qrcode/               POST /qrcode
└── pkg/
    ├── auth/                     JWT ミドルウェア（現在未使用）
    ├── config/                   Firebase / Firestore 初期化
    ├── interpreter/              リクエストボディのデコード
    ├── logger/                   zap ロガー
    ├── presenter/                JSON レスポンス生成
    └── server/                   HTTP サーバー・グレースフルシャットダウン
```
