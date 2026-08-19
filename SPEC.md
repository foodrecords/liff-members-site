# FOOD RECORDS メンバーズ — システム仕様書

> 最終更新: 2026-06-14

---

## 目次

1. [システム概要](#1-システム概要)
2. [アーキテクチャ](#2-アーキテクチャ)
3. [Firestoreデータモデル](#3-firestoreデータモデル)
4. [ポイント設計](#4-ポイント設計)
5. [ランク設計](#5-ランク設計)
6. [QRコード種別](#6-qrコード種別)
7. [チェックインQRコード（TOTP）](#7-チェックインqrコードtotp)
8. [新規登録フロー](#8-新規登録フロー)
9. [APIエンドポイント](#9-apiエンドポイント)
10. [フロントエンド（LIFFアプリ）](#10-フロントエンドliffアプリ)
11. [GAS（スプレッドシート管理）](#11-gasスプレッドシート管理)
12. [Square連携](#12-square連携)
13. [エラーメッセージ一覧](#13-エラーメッセージ一覧)

---

## 1. システム概要

FOOD RECORDS メンバーズ ポイントシステム。LINEミニアプリ（LIFF）を使ったQRコード型ポイント付与システム。

- **スタッフ操作**: Googleスプレッドシート（GAS）からQRコードを発行し、Firestoreに書き込む
- **ユーザー操作**: LINEアプリ内でQRコードをスキャン → ポイント付与
- **特典交換**: 溜まったポイントをアプリ内で特典チケットと交換（消費型）

---

## 2. アーキテクチャ

```
[Google Spreadsheet + GAS]
  └─ シリアルナンバー生成・QRコード出力
  └─ Firestore serials コレクションへ書き込み
  └─ 交換特典カタログを reward_catalog へ同期

[checkin-display.html（店舗タブレット表示）]
  https://members.agaruke.com/checkin-display.html
  └─ TOTP（6時間ウィンドウ）でチェックインQRを動的生成
  └─ 60秒ごとにウィンドウ切り替えを監視・自動更新

[ユーザー（LINEアプリ）]
  └─ LIFF URL を開く
  └─ QRコードをスキャン → ポイント付与
  └─ ポイントを使って特典と交換

[members-site（GitHub Pages）]
  https://members.agaruke.com
  └─ LIFF SDK で LINE 認証
  └─ members-api へリクエスト

[members-api（Cloud Run）]
  https://members-api-438247672947.us-central1.run.app
  └─ LINE API でトークン検証・プロフィール取得
  └─ Firestore でシリアル照合・ポイント更新・特典交換
```

### 技術スタック

| レイヤー | 技術 |
|---------|------|
| フロントエンド | HTML / jQuery / LIFF SDK 2.1 |
| ホスティング | GitHub Pages |
| API サーバー | Go（go-chi） |
| サーバー実行環境 | Cloud Run |
| データストア | Firestore |
| 認証 | LINE アクセストークン（`Authorization: Bearer <token>`） |
| スプレッドシート | Google Apps Script |

---

## 3. Firestoreデータモデル

### 3.1 `serials/{serialCode}`

シリアルナンバー（QRコード）ごとに1ドキュメント。

| フィールド | 型 | 説明 |
|-----------|---|------|
| `point` | int | 付与ポイント数 |
| `type` | string | `"once"` または `"recurring"` |
| `used` | bool | 使用済みフラグ（`once` 型のみ） |
| `used_id` | string | 使用したメンバーの6桁番号（`once` 型のみ） |

- `recurring` 型: `used` / `used_id` フィールドなし。サブコレクション `user_scans/{userId}` でクールダウン管理。

#### `serials/{code}/user_scans/{userId}`（recurring 型のみ）

| フィールド | 型 | 説明 |
|-----------|---|------|
| `last_scanned_at` | timestamp | 最終スキャン日時 |

### 3.2 `members/{userId}`

LINE UserID をドキュメントIDとするメンバー情報。

| フィールド | 型 | 説明 |
|-----------|---|------|
| `number` | int64 | 6桁のメンバー番号（0〜999999 のランダム） |
| `name` | string | LINE 表示名 |
| `point` | int | **現在残高**（交換で減算される） |
| `total_earned_point` | int | **累積獲得ポイント**（ランク計算用・減算しない） |
| `last_checkin_date` | string | 最終チェックイン日（`"2006-01-02"` 形式・JST） |

> **後方互換**: `total_earned_point == 0` かつ `point > 0` の既存メンバーは `point` の値を `total_earned_point` として扱う。

### 3.3 `member_numbers/{paddedNumber}`

メンバー番号の重複防止用インデックス。6桁ゼロ埋めをドキュメントID とする。

| フィールド | 型 | 説明 |
|-----------|---|------|
| `user_id` | string | LINE UserID |

### 3.4 `members/{userId}/coupons/{couponId}`

ユーザーごとのサブコレクション。発行済みクーポン。

| フィールド | 型 | 説明 |
|-----------|---|------|
| `title` | string | 特典名 |
| `description` | string | 説明文 |
| `image_url` | string | 画像URL |
| `reward_id` | string | `reward_catalog` のドキュメントID |
| `point_cost` | int | 交換時に消費したポイント（ウェルカムクーポンは 0） |
| `issued_at` | timestamp | 発行日時 |
| `expires_at` | timestamp | 有効期限（発行から3ヶ月） |
| `used` | bool | 使用済みフラグ |
| `used_at` | timestamp | 使用日時（未使用時はゼロ値） |
| `square_discount_code` | string | Square割引コード（任意） |
| `product_url` | string | モバイルオーダーURL（任意） |

### 3.5 `reward_catalog/{rewardId}`

交換可能な特典カタログ。GAS `syncRewardCatalog()` で同期。

| フィールド | 型 | 説明 |
|-----------|---|------|
| `title` | string | 特典名（例: 基本トッピングチケット） |
| `required_points` | int | 必要ポイント |
| `description` | string | 説明文 |
| `active` | bool | 有効/無効 |
| `image_url` | string | 画像URL |
| `sort_order` | int | 表示順（昇順） |
| `pricing_rule_id` | string | Square Pricing Rule ID（任意） |
| `square_item_id` | string | Square 商品ID（任意・モバイルオーダーURL生成用） |

#### デフォルト特典カタログ

| `reward_id` | 特典名 | 必要ポイント | 表示順 |
|------------|--------|------------|--------|
| `topping_basic` | 基本トッピングチケット | 200 | 1 |
| `topping_deluxe` | 豪華トッピングチケット | 400 | 2 |
| `side_dish` | おかずチケット | 750 | 3 |
| `bento` | お弁当チケット | 1200 | 4 |

### 3.6 `square_pool/{docId}`

Square割引コードの事前プール。`reward_catalog` の `pricing_rule_id` に紐づく。

| フィールド | 型 | 説明 |
|-----------|---|------|
| `code` | string | Square割引コード |
| `pricing_rule_id` | string | 対象の Pricing Rule ID |
| `used` | bool | 使用済みフラグ |
| `used_by` | string | 使用した LINE UserID |
| `used_at` | timestamp | 使用日時 |

---

## 4. ポイント設計

### フィールドの役割

| フィールド | 役割 |
|-----------|------|
| `members.point` | **現在残高**。特典交換で減算。 |
| `members.total_earned_point` | **累積獲得ポイント**。ランク計算に使用。減算しない。 |

### ポイント付与タイミング

| トリガー | 付与ポイント | 制限 |
|---------|------------|------|
| シリアルナンバースキャン（`once`型） | シリアルに設定した値 | 1回のみ使用可 |
| シリアルナンバースキャン（`recurring`型） | シリアルに設定した値 | 3時間クールダウン |
| チェックインQRスキャン（`DL_` prefix） | 100pt | 1日1回（JST基準） |
| 新規登録（ウェルカムクーポン発行） | ポイント付与なし | 最安値の特典を無償発行 |

---

## 5. ランク設計

ランクは `total_earned_point`（累積ポイント）で決まる。ランクダウンなし。

| ランク | 累積ポイント閾値 | カード色 |
|-------|----------------|---------|
| Green | 0 〜 999 | 緑 |
| Bronze | 1,000 〜 2,999 | 茶 |
| Silver | 3,000 〜 7,999 | 銀 |
| Gold | 8,000 〜 | 金 |

### ランクアップ演出

ポイント更新時に旧ランク < 新ランクだった場合、フロントエンドでランクアップトーストとカードアニメーションを表示。

---

## 6. QRコード種別

### 6.1 シリアルナンバー QR（スプレッドシート発行）

- コード形式: `FRXXXXXXXX`（FR + 大文字英数字8文字）
- LIFF URL: `https://liff.line.me/{LIFF_ID}/?code=FRXXXXXXXX`
- QR生成: quickchart.io API（中央にアイコン埋め込み対応）

| type | 挙動 |
|------|------|
| `once` | 先着1名のみ使用可。使用後 `used: true` に更新。 |
| `recurring` | 複数人・複数回使用可。ユーザーごとに3時間クールダウン。 |

> **テストコード**: `FR1234567890` はシリアルの `used` フラグを更新しない（検証用）。

### 6.2 チェックインQR（店舗タブレット表示）

- コード形式: `DL_{12桁HEXトークン}`
- 店舗の `checkin-display.html` で動的に生成・表示
- TOTPアルゴリズムで6時間ウィンドウごとに更新（詳細は次節）

---

## 7. チェックインQRコード（TOTP）

### トークン生成アルゴリズム

```
window = floor(unix_timestamp / 21600)  // 21600秒 = 6時間
input  = BigEndian uint64(window)
token  = hex(HMAC-SHA256(CHECKIN_SECRET, input))[:12]
QR URL = https://liff.line.me/{LIFF_ID}?code=DL_{token}
```

### 更新タイミング（JST）

`3:00 / 9:00 / 15:00 / 21:00` の4回/日。

### バリデーション（サーバー側）

現在ウィンドウ ± 3分のトークンを有効とする（ポーリング遅延・タイマーズレの許容）。

### 1日1回制限

`members.last_checkin_date`（`"YYYY-MM-DD"` JST）と当日日付を比較。同じ場合は `400 ALREADY_CHECKED_IN`。

### 店舗表示ページ

`checkin-display.html` はブラウザ上でトークンを生成（Web Crypto API）。
- URLハッシュ `#secret=...` でシークレット自動入力
- `checkin-bg.webp` が存在すれば画像ベースレイアウト、なければCSSフォールバック
- 60秒ごとにウィンドウ切り替えを監視し、切り替わり時に自動更新
- 「今すぐ更新」ボタンで手動リフレッシュ可

---

## 8. 新規登録フロー

`GET /members` または `POST /qrcode` でメンバーが未登録の場合に自動実行。

1. LINE プロフィール取得（`name`）
2. `member_numbers` で重複なしの6桁乱数を生成
3. `members/{userId}` を作成（`point: 0`, `total_earned_point: 0`）
4. `member_numbers/{paddedNumber}` にインデックス追加
5. **ウェルカムクーポン発行**: `reward_catalog` から `active: true` かつ最小 `required_points` の特典を動的取得し、`point_cost: 0` でクーポンを発行。`pricing_rule_id` がある場合は Square コードもセット。
6. 上記をFirestore Batch Write で原子的にコミット

---

## 9. APIエンドポイント

### 認証

全エンドポイント（`/admin/*` を除く）で `Authorization: Bearer {LINE_ACCESS_TOKEN}` が必要。
サーバーは LINE API `GET https://api.line.me/v2/profile` でトークン検証とプロフィール取得を行う。

### レスポンス形式

```json
{
  "message": "ok",
  "data": { ... }
}
```

エラー時:

```json
{
  "message": "エラーメッセージ"
}
```

---

### GET /members

メンバー情報を取得。メンバー未登録の場合は新規登録してから返す。

**レスポンス**

```json
{
  "number": "FR000042",
  "name": "山田太郎",
  "point": 850,
  "total_earned_point": 1050,
  "rank": "bronze",
  "next_rank": "silver",
  "next_rank_point": 1950,
  "is_new_member": true
}
```

| フィールド | 説明 |
|-----------|------|
| `number` | `FR` + 6桁ゼロ埋め |
| `point` | 現在残高 |
| `total_earned_point` | 累積獲得ポイント |
| `rank` | `green` / `bronze` / `silver` / `gold` |
| `next_rank` | 次ランク（Goldの場合は省略） |
| `next_rank_point` | 次ランクまでの残りポイント（Goldの場合は省略） |
| `is_new_member` | 新規登録時のみ `true` |

---

### POST /qrcode

QRコードをスキャンしてポイントを付与する。

**リクエスト**

```json
{ "code": "FRABCD1234" }
```

**コード種別別処理**

| コード形式 | 処理 |
|-----------|------|
| `DL_{token}` | チェックイン処理（TOTP検証 → 100pt付与） |
| `once` 型シリアル | 未使用確認 → ポイント付与 → `used: true` |
| `recurring` 型シリアル | 3時間クールダウン確認 → ポイント付与 |

**レスポンス**

```json
{
  "number": "FR000042",
  "get_point": 100,
  "point": 950,
  "total_earned_point": 1150,
  "rank": "bronze",
  "next_rank": "silver",
  "next_rank_point": 1850
}
```

---

### GET /coupons

ユーザーの獲得済みクーポン一覧を返す。

**レスポンス**

```json
{
  "coupons": [
    {
      "id": "abc123",
      "title": "基本トッピングチケット",
      "description": "店舗スタッフにこの画面をご提示ください。",
      "image_url": "https://...",
      "reward_id": "topping_basic",
      "point_cost": 200,
      "issued_at": "2026-06-01T10:00:00Z",
      "expires_at": "2026-09-01T10:00:00Z",
      "used": false,
      "product_url": "https://food-records.square.site/?item=XXX&cc=YYY"
    }
  ]
}
```

---

### POST /coupons/{id}/use

クーポンを使用済みにする。

- 既に `used: true` の場合は `400 ALREADY_USED`
- `used: true`, `used_at: now` を Firestore に書き込み

**レスポンス**: `{ "message": "ok" }`

---

### GET /rewards

交換可能な特典カタログを返す（`active: true` のみ、`sort_order` 昇順。同値の場合は `required_points` 昇順）。

**レスポンス**

```json
[
  {
    "id": "topping_basic",
    "title": "基本トッピングチケット",
    "required_points": 200,
    "description": "店舗スタッフにこの画面をご提示ください。",
    "image_url": "https://...",
    "sort_order": 1
  }
]
```

---

### POST /rewards/{id}/exchange

ポイントを消費して特典クーポンを発行する。Firestore トランザクションで原子的に実行。

**処理順序**

1. `reward_catalog/{id}` を取得（`active: true` 確認）
2. `square_pool` から未使用コードを検索（`pricing_rule_id` がある場合）
3. `members/{userId}.point` を確認（残高不足なら `400 INSUFFICIENT_POINTS`）
4. **Firestoreトランザクション（全 read → 全 write）:**
   - `members/{userId}.point -= required_points`（`total_earned_point` は変更しない）
   - `square_pool` の該当コードを `used: true` にマーク
   - `members/{userId}/coupons/{newId}` にクーポン発行（有効期限3ヶ月）

**レスポンス**

```json
{
  "coupon_id": "xyz789",
  "title": "基本トッピングチケット",
  "point_cost": 200,
  "new_point": 650,
  "square_discount_code": "ABC123",
  "product_url": "https://food-records.square.site/?item=XXX&cc=ABC123"
}
```

**エラー**

| 条件 | HTTPステータス | メッセージ |
|------|--------------|-----------|
| 残高不足 | 400 | `ポイントが不足しています（必要: {N}pt）` |
| 特典が存在しない | 400 | `特典が見つかりません` |
| 特典が無効 | 400 | `この特典は現在ご利用いただけません` |

---

### GET /admin/pool-monitor

Square コードプールの残数を確認する（管理用）。

### POST /admin/square-coupons

Square APIを通じてクーポンを作成する（管理用）。

---

## 10. フロントエンド（LIFFアプリ）

### ページ構成

| URL | 説明 |
|-----|------|
| `https://members.agaruke.com/` | メインアプリ（ポイントカード） |
| `https://members.agaruke.com/checkin-display.html` | 店舗タブレット向けチェックインQR表示 |

### メインアプリの構成

```
メンバーシップカード
  ├─ ブランド名（AGARUKE POINT CARD）
  ├─ 現在のポイント残高
  ├─ ランクバッジ（GREEN / BRONZE / SILVER / GOLD）
  ├─ ランクプログレスチャート（SVG円グラフ）
  │   └─ 累積ポイント基準で次ランクまでの進捗を表示
  ├─ メンバー名
  └─ メンバー番号（FR000042形式）

タブ
  ├─ 「獲得済みの特典」（デフォルト表示）
  │   └─ クーポンカード一覧（未使用→期限切れ→使用済み の順）
  └─ 「ポイントを使う」
      └─ 交換カタログ一覧（残高不足はボタングレーアウト）

[QRコードをスキャン] 固定ボタン（下部）
```

### LIFF 初期化フロー

```
initializeLiff()
  ├─ LINE 未ログイン → ログインダイアログ表示
  └─ ログイン済み
      ├─ showPoint()      → GET /members → カード表示
      │   └─ 完了後 checkCode() → POST /qrcode（URLパラメータに code がある場合）
      ├─ showCoupons()    → GET /coupons → 獲得済みタブ表示
      └─ showRewards()    → GET /rewards → 交換タブ表示
```

> **レースコンディション対策**: `GET /members` と `POST /qrcode` を並列実行すると新規メンバーが二重登録される可能性があるため、`showPoint` 完了コールバック内で `checkCode` を実行する。

### URLパラメータ処理

QRコードのLIFF URLは `?code=FRXXXXXXXX` を含む。`liff.state` 経由でパラメータが渡される場合にも対応。

### クーポンカード

- 未使用: 通常表示
- 新規取得（交換直後）: `NEW` バッジ
- 期限切れ: グレー表示（`coupon-card--expired`）
- 使用済み: グレー表示 + `使用済み` バッジ

### クーポン詳細モーダル（ボトムシート）

- 画像・タイトル・説明・有効期限を表示
- **「店舗で使用する」**: 確認ダイアログ → `POST /coupons/{id}/use`
- **「モバイルオーダーで使用する」**: `product_url` がある場合のみ表示。LINEアプリ内ブラウザで開く。
- 交換カタログからモーダルを開いた場合は「{N}pt で交換する」ボタンを表示

### トースト一覧

| トースト | 表示条件 | 表示時間 |
|---------|---------|---------|
| ポイント獲得（緑） | QRスキャン成功 | 2.4秒 |
| ウェルカム（ピンク） | 新規登録 | 4.2秒 |
| クーポン獲得（緑） | 交換成功 | 3.2秒 |
| ランクアップ | ランク上昇 | 4.5秒 |
| エラーバナー（赤） | APIエラー | 3.5秒 |

---

## 11. GAS（スプレッドシート管理）

### Script Properties（必須設定）

| プロパティ名 | 説明 |
|-----------|------|
| `FIREBASE_PROJECT_ID` | Firebase プロジェクトID |
| `SERVICE_ACCOUNT_JSON` | サービスアカウントのJSONキー（全文） |
| `LIFF_ID` | LINE LIFF アプリID（例: `1234567890-abcdefgh`） |

### カスタムメニュー

スプレッドシートを開くと「シリアル管理」メニューが追加される。

| メニュー項目 | 関数 |
|-----------|------|
| シリアルナンバー生成 | `generateSerials()` |
| 交換特典を読み込み（Firestore → シート） | `loadRewardCatalog()` |
| 交換特典を同期（シート → Firestore） | `syncRewardCatalog()` |
| 交換特典シートを作成 | `createRewardCatalogSheet()` |

### シリアルナンバー生成シート

アクティブシートの2行目以降に設定を記述して `generateSerials()` を実行。

| 列 | 内容 | 例 |
|----|------|----|
| A | 生成枚数 | `50` |
| B | 付与ポイント | `100` |
| C | タイプ | `once` または `recurring` |

生成結果は「生成結果」シートに出力（シリアルナンバー・ポイント・タイプ・生成日時・QRコード画像）。

**コード形式**: `FR` + 大文字英数字8文字（例: `FRABCD1234`）
**QR URL**: `https://liff.line.me/{LIFF_ID}/?code={CODE}` を quickchart.io でQR化

### 交換特典シート（「交換特典」シート名）

| 列 | 内容 | 例 |
|----|------|----|
| A | 定義ID | `topping_basic` |
| B | タイトル | 基本トッピングチケット |
| C | 必要ポイント | `200` |
| D | 説明文 | 店舗スタッフにこの画面をご提示ください |
| E | 有効 | `TRUE` / `FALSE` |
| F | 画像URL | `https://...` |
| G | 表示順 | `1` |
| H | Square Pricing Rule ID | `3KKXKOB7LLN5CFOLIXDXFXIZ`（省略可） |
| I | Square 商品ID | `TTXDXT77BJJIAGB5GKJ5MMHC`（省略可） |

**同期の向き**
- `syncRewardCatalog()`: シート → Firestore（上書き）
- `loadRewardCatalog()`: Firestore → シート（確認ダイアログあり）

### Firestore アクセス方法

GAS ではサービスアカウントJWTで OAuth2 アクセストークンを取得し、Firestore REST API を呼び出す（最大500件/リクエストのバッチ書き込み）。

---

## 12. Square連携

### square_pool（事前コードプール）

特典交換時に Square 割引コードを自動発行するため、事前にコードを `square_pool` コレクションへ登録しておく。

- `reward_catalog.pricing_rule_id` が設定されている特典のみ Square コード発行対象
- 交換時にトランザクション内で未使用コードを1件取得・使用済みマーク
- プールが空（`ErrPoolEmpty`）の場合はクーポン自体は発行するが Square コードなし（ログ出力）

### product_url（モバイルオーダー連携）

`reward_catalog.square_item_id` が設定されている場合、Square コード発行成功時に以下のURLを生成：

```
https://food-records.square.site/?item={square_item_id}&cc={discount_code}
```

ユーザーはモバイルオーダーURLからオンライン注文時に割引を適用できる。

### 管理エンドポイント

| エンドポイント | 説明 |
|--------------|------|
| `POST /admin/square-coupons` | Square API 経由でクーポンを作成 |
| `GET /admin/pool-monitor` | プール残数を確認 |

---

## 13. エラーメッセージ一覧

| コード/メッセージ | 条件 |
|----------------|------|
| `INVALID_TOKEN` | LINE アクセストークンが無効 |
| `INVALID_BODY` | リクエストボディのパースエラー |
| `シリアルナンバーが見つかりません` | 存在しないシリアルコード |
| `このシリアルナンバーは既に使われています` | `once` 型のシリアルを再使用 |
| `本日はすでにスキャン済みです` | チェックイン1日1回制限 |
| `次のスキャンまで{N}時間{M}分お待ちください` | `recurring` 型クールダウン中 |
| `QRコードの有効期限が切れています。店舗のQRコードをスキャンしてください` | TOTP検証失敗 |
| `ポイントが不足しています（必要: {N}pt）` | 残高不足で交換不可 |
| `特典が見つかりません` | `reward_catalog` に該当IDなし |
| `この特典は現在ご利用いただけません` | `active: false` の特典 |
| `メンバー情報が見つかりません` | 交換時にメンバー未登録 |
| `クーポンが見つかりません` | 使用するクーポンが存在しない |
| `このクーポンは既に使用済みです` | 使用済みクーポンを再使用 |
| `FAILED_CREATE_USER` | 新規メンバー登録のコミット失敗 |
