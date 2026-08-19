# agaruke-members — 開発コンテキスト

## システム概要

FOOD RECORDS メンバーズ ポイントシステム。
LINEミニアプリ（LIFF）を使ったQRコード型ポイント付与システム。
スタッフがスプレッドシートからQRコードを発行し、ユーザーがLINEアプリでスキャンするとポイントが付与される。
溜まったポイントはアプリ内で特典チケットと交換できる（消費型）。

## アーキテクチャ

```
[Google Spreadsheet + GAS]
  └─ シリアルナンバー生成・QRコード出力
  └─ Firestore serials コレクションへ書き込み
  └─ 交換特典カタログを reward_catalog へ同期

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

## リポジトリ構成

```
agaruke-members/
├── gas/              Google Apps Script（シリアル生成・交換特典同期）
├── members-site/     LIFF フロントエンド（GitHub Pages）
└── members-api/      Go API サーバー（Cloud Run）
```

## Firestore コレクション構造

| コレクション | ドキュメントID | フィールド |
|---|---|---|
| `serials` | シリアルコード（例: `FRABCD1234`） | `point: int`, `used: bool`, `used_id: string`, `type: string` |
| `members` | LINE UserID | `number: int64`, `name: string`, `point: int`, `total_earned_point: int`, `last_checkin_date: string` |
| `member_numbers` | 6桁のメンバー番号（例: `000042`） | `user_id: string` |
| `reward_catalog` | 定義ID（例: `topping_basic`） | `title: string`, `required_points: int`, `description: string`, `active: bool`, `image_url: string`, `sort_order: int` |
| `members/{userId}/coupons` | クーポンID（自動生成） | `title: string`, `reward_id: string`, `point_cost: int`, `issued_at: timestamp`, `used: bool`, `used_at: timestamp`, `image_url: string`, `description: string` |

## ポイント設計

- `members.point` — **現在残高**（交換で減算される）
- `members.total_earned_point` — **累積獲得ポイント**（ランク計算用・減算しない）
- **新規登録ボーナス**: 200pt
- **チェックインボーナス**: 100pt/回（1日1回）

既存メンバー（`total_earned_point` 未設定）は `point` の値を `total_earned_point` として扱う（後方互換）。

## ランク設計

| ランク | `total_earned_point` |
|--------|---------------------|
| Green  | 0〜999 |
| Bronze | 1000〜2999 |
| Silver | 3000〜7999 |
| Gold   | 8000〜19999 |
| Secret   | 20000〜 |

ランクダウンなし。`total_earned_point` 基準なのでポイント消費でランクは下がらない。

## 交換特典（デフォルト値）

| 必要ポイント | 特典名 |
|-------------|--------|
| 200pt | 基本トッピングチケット |
| 400pt | 豪華トッピングチケット |
| 750pt | おかずチケット |
| 1200pt | お弁当チケット |

特典内容はスプレッドシートの「交換特典」シートから GAS `syncRewardCatalog()` で `reward_catalog` コレクションに同期。

## API エンドポイント

| メソッド | パス | 説明 |
|---|---|---|
| `GET` | `/members` | メンバー情報（point, total_earned_point, rank） |
| `POST` | `/qrcode` | QRスキャン・ポイント付与（DL_xxx でチェックイン） |
| `GET` | `/coupons` | 獲得済みクーポン一覧 |
| `POST` | `/coupons/{id}/use` | クーポン使用 |
| `GET` | `/rewards` | 交換カタログ一覧 |
| `POST` | `/rewards/{id}/exchange` | ポイント消費してクーポン発行（Firestore トランザクション） |
