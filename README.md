# agaruke-members

FOOD RECORDS メンバーズ ポイントシステム

## システム概要

LINEミニアプリ（LIFF）を使ったQRコード型ポイント付与システム。
スタッフがスプレッドシートからQRコードを発行し、ユーザーがLINEアプリでスキャンするとポイントが付与される。

## アーキテクチャ

```
[Google Spreadsheet + GAS]
  └─ シリアルナンバー生成・QRコード出力
  └─ Firestore serials コレクションへ書き込み

[ユーザー（LINEアプリ）]
  └─ LIFF URL を開く
  └─ QRコードをスキャン
  └─ ポイント付与結果を表示

[members-site（GitHub Pages）]
  https://members.agaruke.com
  └─ LIFF SDK で LINE 認証
  └─ members-api へリクエスト

[members-api（Cloud Run）]
  https://members-api-438247672947.us-central1.run.app
  └─ LINE API でトークン検証・プロフィール取得
  └─ Firestore でシリアル照合・ポイント更新
```

## ポイント付与フロー

```

1. スタッフ: スプレッドシートに count・point を入力
            → GAS「シリアル管理 > シリアルナンバー生成」を実行
            → Firestore serials/{code} に {point, used: false} で保存
            → 「生成結果」シートにシリアルナンバーと QR コード画像が出力される

2. ユーザー: LINE アプリから LIFF URL を開く
            → LINE ログイン（未ログインの場合）

3. ユーザー: 「QRコードを読み取る」ボタンをタップ
            → liff.scanCodeV2() でカメラ起動・QR スキャン

4. members-site: POST /qrcode { code: "FRXXXXXXXX" } を送信

5. members-api:
   a. Firestore serials/{code} を取得
   b. used: true なら 400 エラー
   c. LINE アクセストークンで /v2/profile を呼び出しユーザー特定
   d. members/{userId} のポイントを加算（未登録なら新規作成）
   e. serials/{code} を used: true にマーク（バッチ書き込み）
   f. 付与後ポイントを返却

6. ユーザー: 画面に「XXX point get!」と現在ポイントが表示される
```

## セルフレジ連携

- kioskが2分有効の連携QRを表示し、LIFFの既存「QRコードを読み取る」でスキャンしてLINE会員を紐づける。
- kiosk APIはサービス間認証で連携状態を確認し、会員と利用可能クーポンを解決する。QRに会員情報や認証情報は含めない。
- クーポンは会計セッションへ10分予約し、注文確定時に使用済みへ変更する。キャンセル時は利用可能へ戻す。
- 注文確定時に`point`と`total_earned_point`へ100ポイントを冪等加算する。ポイント交換によるクーポン発行は従来どおり利用できる。
- members APIには秘密値`MEMBERS_SERVICE_KEY`を設定し、kiosk APIと同じ値を使用する。

## Firestore コレクション構造

| コレクション | ドキュメントID | フィールド |
|---|---|---|
| `serials` | シリアルコード（例: `FRABCD1234`） | `point: int`, `used: bool`, `used_id: string` |
| `members` | LINE UserID | `number: int64`, `name: string`, `point: int` |
| `member_numbers` | 6桁のメンバー番号（例: `000042`） | `user_id: string` |

## リポジトリ構成

```
agaruke-members/
├── gas/              Google Apps Script（シリアル生成・QRコード出力）
├── members-site/     LIFF フロントエンド（GitHub Pages）
├── members-api/      Go API サーバー（Cloud Run）
└── square-session/   Square管理画面セッションの補助サービス
```

各ディレクトリの詳細は配下の README を参照。

## GitHub Pages

`master`へ`members-site/**`の変更をpushすると、GitHub Actionsが`members-site/`だけをPagesへ公開する。APIコード、環境変数、認証情報は公開成果物へ含めない。カスタムドメインは`members-site/CNAME`で管理する。
