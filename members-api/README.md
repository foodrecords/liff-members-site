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

### DELETE /members

LINE認証済み会員を即時退会状態にする。本文に`{"confirmation":"DELETE"}`が必要。会員・ポイント・クーポンは復旧用として内部で30日間保持する。通常の`GET /members`は退会状態を解除せず`ACCOUNT_DELETED`を返し、利用者が明示的に`POST /members/restore`を実行した場合だけ元の会員番号とデータを復旧する。期限到来後の完全削除は、本番導入時に定期ジョブとして有効化する。

### POST /members/register

モバイルオーダーの簡易規約で明示同意した場合だけ会員を作成する。規約・プライバシーポリシーの現行バージョンと`consent_source=mobile_order_liff`を要求し、会員ドキュメントへバージョン、同意日時、同意元を保存する。退会から30日以内の同じLINE会員は、この操作で既存データを復旧する。通常の`GET /members`は未登録会員を自動作成せず`MEMBER_REGISTRATION_REQUIRED`を返す。

## ローカル開発

### 必要なもの

- Go 1.21+
- Docker

### 起動

orderecのFirebase Emulatorを使う場合：

```bash
make emulator-dev
```

従来のFirebaseプロジェクトへ接続するDocker環境を使う場合：

```bash
# 環境変数ファイルをコピー
cp etc/docker/.env.example etc/docker/.env
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
| `ORGANIZATION_UUID` | 会員データの契約主体。ローカル既定値は`35095fe0-1efc-40ff-bd13-9720c6d09e0f` |
| `MEMBERS_DATA_LAYOUT` | `organization`で`organizations/{organization_uuid}`配下を使用。未設定時は本番互換の従来パス |
| `LINE_LOGIN_CHANNEL_ID` | organizationに対応するLINE Login Channel ID。アクセストークン検証結果の`client_id`と照合する |

Firestore Emulatorの従来パスをorganization配下へコピーする場合は次を実行する。旧データは削除しない。このコマンドは`FIRESTORE_EMULATOR_HOST`未設定時には停止する。

```bash
ENV=local PROJECT_ID=demo-orderec \
FIRESTORE_EMULATOR_HOST=127.0.0.1:8085 \
ORGANIZATION_UUID=35095fe0-1efc-40ff-bd13-9720c6d09e0f \
go run ./cmd/migrate-organization
```

本番では会員カードがFirebase／Firestoreプロジェクト`fr-agaruke`、orderec・モバイルオーダーが`food-records-prod`に分かれている。現在の移行コマンドはEmulator専用であり、本番統合には使用しない。本番実装時は件数・ポイント・クーポン・会員番号の整合確認、差分同期、切替、ロールバックを備えた専用マイグレーションを別途実装する。

## セルフレジ連携API

- `POST /kiosk/checkouts/link`: LIFFがLINEアクセストークン付きでKiosk表示QRのトークンを会員へ紐づける。
- `POST /kiosk/members/resolve-line`: orderec内部APIがLINEアクセストークンを検証し、organization内の会員情報を解決する。
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
docker build -f etc/docker/api/Dockerfile.prod -t gcr.io/fr-agaruke/members-api .
docker push gcr.io/fr-agaruke/members-api

# Cloud Run にデプロイ
gcloud run deploy members-api \
  --platform managed \
  --image gcr.io/fr-agaruke/members-api \
  --region us-central1 \
  --allow-unauthenticated
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
