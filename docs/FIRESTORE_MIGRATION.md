# 会員Firestore本番移行手順

## 目的と境界

`fr-agaruke`直下の会員カードデータを、`food-records-prod`の
`organizations/35095fe0-1efc-40ff-bd13-9720c6d09e0f`配下へ移す。

移行コマンドは次の3モードを持つ。

- `inventory`: 両プロジェクトを読み取り、件数・整合性・ダイジェストを出す。書き込みなし。
- `copy`: 永続データを再実行可能な形でコピーし、直後に再読込して検証する。
- `verify`: 両側を読み取り、欠損・余分・内容不一致を検査する。書き込みなし。

レポートには個人情報やドキュメントIDを出さない。不一致箇所はパスの短いSHA-256だけを出す。

## 移行対象

次の永続データは、全サブコレクションを再帰的にコピーする。

- `members`（`coupons`、`point_logs`を含む）
- `member_numbers`
- `reward_catalog`
- `serials`（`user_scans`を含む）
- `square_pool`

次の一時状態は棚卸しだけ行い、コピーしない。

- `kiosk_member_tokens`
- `kiosk_coupon_reservations`

一時状態は切替前に新規発行を止め、既存Kiosk会計が完了または期限切れになって0件となったことを確認する。進行中会計を別プロジェクトへ複製しない。

## 安全条件

- 読み取り用主体には両プロジェクトのFirestore閲覧権限だけを付与する。
- コピー時だけ、移行主体へ移行先organization配下の書込権限を付与する。
- `copy`は`--apply`と完全一致する`MEMBERS_MIGRATION_CONFIRM`がなければ停止する。
- 移行先に同一パス・同一内容があればスキップするため、同じコピーを再実行できる。
- 移行先に同一パス・異なる内容がある場合は既定で停止する。
- Firestore DocumentReference型を検出した場合は、参照先プロジェクトの変換方針を決めるまでコピーを停止する。
- 旧データは削除しない。検証完了後もロールバック期間中は読み取り可能な状態で保持する。

## 事前棚卸し

Application Default Credentialsを用意し、Git管理外のローカル作業ディレクトリへレポートを保存する。

```bash
mkdir -p /private/tmp/members-firestore-migration

cd /Users/saku/39labo/agaruke-members/members-api
go run ./cmd/migrate-production \
  --mode=inventory \
  --report=/private/tmp/members-firestore-migration/inventory.json
```

確認項目:

- collection別ドキュメント件数
- 会員数と会員番号index数
- 会員番号の欠損、不一致、孤立、同一会員へ向く重複index
- 現在ポイント合計と累積獲得ポイント合計
- クーポン総数、使用済み数、存在しない特典参照
- ポイントログ数、recurringシリアル利用記録数
- DocumentReference型の有無
- Kiosk一時状態の件数

## リハーサル

本番コピー前に、同じコマンドをEmulatorまたは専用検証プロジェクトで実行する。本番プロジェクトを指定するリハーサルでは、移行先を本番organizationにしない。

```bash
MEMBERS_MIGRATION_CONFIRM='SOURCE_PROJECT->TARGET_PROJECT/TEST_ORGANIZATION' \
go run ./cmd/migrate-production \
  --mode=copy \
  --source-project=SOURCE_PROJECT \
  --target-project=TARGET_PROJECT \
  --organization=TEST_ORGANIZATION \
  --apply \
  --report=/private/tmp/members-firestore-migration/rehearsal-copy.json
```

## 2026-08-28事前棚卸し結果

読み取り専用`inventory`を本番2プロジェクトへ実行した。レポートはGit管理外の`/private/tmp/members-firestore-migration/inventory-20260828.json`に保存した。

| 項目 | 旧`fr-agaruke` | 新organization |
| --- | ---: | ---: |
| 永続データ | 281 | 0 |
| members | 19 | 0 |
| member_numbers | 28 | 0 |
| reward_catalog | 4 | 0 |
| serials | 30 | 0 |
| square_pool | 200 | 0 |
| ポイント残高合計 | 1,400 | 0 |
| 累積獲得ポイント合計 | 1,400 | 0 |

追加確認:

- 会員自身が持つ現在番号について、index欠損・参照不一致・存在しない会員への参照は0件。
- 同一会員へ複数の`member_numbers`が向く重複indexが9件ある。移行では原本一致のため保持し、切替後に別作業として削除可否を確認する。
- クーポン、ポイントログ、recurring serialのuser scanは現時点で0件。
- Squareクーポンプールは200件中32件使用済み。
- serialは30件中20件使用済み。
- Kiosk tokenは総数1,267件中、有効期限内1件。クーポン予約は総数23件、有効期限内0件。
- Firestore DocumentReference型は0件で、プロジェクトをまたぐ参照変換は不要。
- 新organizationは空で、現時点のパス競合はない。

棚卸し時点から切替までの更新は取り込まれないため、本番コピー直前に書き込みを止めて再度inventoryを取得する。

## 2026-08-28本番コピー結果

- `gs://fr-agaruke-firestore-backups/2026-08-28T2048JST-pre-org-migration`へFirestore全体のmanaged exportを取得した。exportは`SUCCESSFUL`、処理対象は1,618文書。専用バケットは`us-central1`、uniform bucket-level access、30日保持。
- 永続データ281文書を`food-records-prod/organizations/35095fe0-1efc-40ff-bd13-9720c6d09e0f`配下へコピーした。
- コピー直後の再読込と、別実行の`verify`の双方で、欠損0、余分0、内容不一致0を確認した。
- 会員19、会員番号index 28、特典4、シリアル30、Square pool 200、ポイント残高合計1,400、累積獲得ポイント合計1,400が一致した。
- 旧`fr-agaruke`のデータは削除していない。Kiosk token・クーポン予約もコピーしていない。
- members API、members-site、GAS、Kiosk、モバイル注文の参照先切替は未実施。旧側では有効Kiosk tokenが1件継続しているため、接続切替前に発行停止と失効確認が必要。

## 2026-09-05差分再取得・本番コピー結果

- 前回コピー後の変更を取り込むため、本番2プロジェクトを再棚卸しした。旧側は永続データ285文書、移行先は281文書で、移行先にのみ存在する文書は0件だった。
- 旧側は会員21、会員番号index 30、特典4、シリアル30、Square pool 200、ポイント残高合計1,600、累積獲得ポイント合計1,600。会員番号の欠損・不一致・孤立、DocumentReference、有効Kiosk token、有効クーポン予約はいずれも0件だった。
- `gs://fr-agaruke-firestore-backups/2026-09-05T0923JST-pre-org-migration-refresh`へ最新のmanaged exportを取得した。operationは`SUCCESSFUL`で完了した。
- 旧側を正として、移行先へ差分7文書（新規4、内容更新3）を反映し、一致済み278文書をスキップした。旧データと一時状態は削除していない。
- コピー直後の再読込と独立`verify`の双方で、永続データ285文書、欠損0、余分0、内容不一致0を確認した。個人情報を含まないレポートはGit管理外の`/private/tmp/members-firestore-migration-20260905/`へ保存した。
- この時点ではmembers API、members-site、GAS、Kiosk、モバイル注文の参照先切替と`MEMBERS_DATA_LAYOUT=organization`の本番有効化は行っていない。旧側が引き続き書込先のため、切替直前に再度inventory／差分copy／verifyを行う。

## 2026-09-05 members API・members site切替

- 切替用イメージを固有タグで作成し、旧revision `members-api-00036-h6s`をロールバック先として保持した。
- Cloud Run実行サービスアカウントへ`food-records-prod`の`roles/datastore.user`を付与した。
- 切替直前に差分copyを再実行し、永続データ285文書の一致を確認した。
- members API revision `members-api-00037-9dd`を100%配信し、`PROJECT_ID=food-records-prod`、対象organization、`MEMBERS_DATA_LAYOUT=organization`へ切り替えた。ヘルスチェックHTTP 200、未認証`GET /members`の`INVALID_TOKEN`応答を確認した。
- members siteへ初回登録の規約・プライバシーポリシー同意UIと`POST /members/register`を追加した。既存会員は従来どおり`GET /members`で表示し、未登録者だけ登録画面へ進む。
- 旧`fr-agaruke`のデータは削除せず、ロールバック用に保持する。GAS、Kiosk、モバイルオーダーAPIの切替は別工程。
- 初回切替revisionで`LINE_LOGIN_CHANNEL_ID`が未設定だったため、members siteの実LIFFアクセスが`INVALID_TOKEN`になった。2026-09-05に本番LIFF IDと対応するChannel IDをCloud Runへ設定し、revision `members-api-00038-c5n`を100%配信した。ヘルスチェックHTTP 200、Firestore権限エラー・panicなしを確認した。
- Kiosk API `orderec-kiosk-api-00046-6xv`は切替前から同じmembers API本番URLを参照し、`MEMBERS_SERVICE_KEY`もmembers APIと一致していた。会員・トークン・クーポン処理はmembers API経由のため、members APIのorganization layout切替によりKiosk側の保存先も同時に新organizationへ切り替わる。Kiosk APIの再デプロイは不要。共有キー付きの無書込対照試験で`INVALID_BODY`まで到達し内部認証成功、`api.kiosk.orderec.com`のヘルスチェックHTTP 200、直近エラーログなしを確認した。

## 2026-09-05 会員サブコレクション再移行

- 利用者確認を受けてFirestore collection groupを直接照会し、旧側に`members/{id}/coupons` 21件が存在する一方、新organization側は0件だったことを確認した。
- 原因は、マイグレーションツールがcollection groupのDocumentRefフルリソースパスを相対パスとして比較していたこと。`coupons`だけでなく`point_logs` 13件と`serials/{id}/user_scans` 1件も集計・コピーから漏れていた。
- フルリソース名と相対パスの双方を正規化する修正・回帰テストを追加。切替後の新環境にポイント100・ポイント履歴1件が追加済みだったため、既存文書を上書きしない`--missing-only`モードを追加した。
- 欠けていた35文書を追加し、新環境でクーポン21件（使用済み3件）、ポイント履歴14件（旧13件＋切替後1件）、QR利用履歴1件を確認。クーポンは独立REST比較で欠損0、余分0、内容不一致0。
- 旧側は永続データ320件、新側は321件。新側だけのポイント履歴1件と会員ポイント差分100は切替後の正常な利用実績として保持し、旧データで上書きしていない。
- 旧直下の`coupon_definitions` 12件は全件有効・期限内だが、現行実行コードから参照されず、`REQUIREMENTS.md`でも廃止済みと定義されているため移行対象外とした。現行の特典定義は`reward_catalog` 4件、発行済みクーポンは会員配下21件を使用する。

## 本番切替

1. `fr-agaruke`のFirestore exportを取得し、復元先と保持期間を記録する。
2. members-siteの新規登録、GASのシリアル生成・同期、Kiosk会員連携、ポイント・クーポン更新を短時間停止する。
3. `active_kiosk_member_tokens`と`active_kiosk_coupon_reservations`が0件であることを棚卸しで確認する。TTL遅延等による期限切れ文書の残存は切替を妨げない。
4. 停止後の最終`inventory`を取得する。
5. 次のコマンドでコピーする。実行前にプロジェクト名とorganizationを声出し確認する。

```bash
MEMBERS_MIGRATION_CONFIRM='fr-agaruke->food-records-prod/35095fe0-1efc-40ff-bd13-9720c6d09e0f' \
go run ./cmd/migrate-production \
  --mode=copy \
  --apply \
  --report=/private/tmp/members-firestore-migration/production-copy.json
```

6. 独立した`verify`を実行する。

```bash
go run ./cmd/migrate-production \
  --mode=verify \
  --report=/private/tmp/members-firestore-migration/production-verify.json
```

7. 件数、ダイジェスト、ポイント合計、会員番号整合、不一致0件を確認する。
8. Firestore index・TTL・IAM・Secret Managerを確認する。
9. members APIのFirestore接続先を`food-records-prod`へ変更し、`ORGANIZATION_UUID`を設定する。
10. `MEMBERS_DATA_LAYOUT=organization`を有効化する。
11. members-site、Kiosk、モバイル注文を限定利用者で結合試験する。
12. 問題がなければ書き込み停止を解除する。

## 結合試験

- 既存会員の会員番号・ポイント・ランク表示
- 新規登録と規約同意保存
- QRシリアルによるポイント付与と冪等性
- recurring QRのクールダウン
- クーポン交換、予約、取消、利用確定
- Kiosk会員連携と注文完了ポイント
- モバイル注文、注文履歴、保存カード
- 退会と30日以内の復旧
- LINE通知と注文詳細リンク

## ロールバック

次のいずれかが発生したら新規書き込みを再停止し、旧members API revisionへ戻す。

- 会員・会員番号・ポイント・クーポンの不一致が1件以上
- 既存会員が未登録扱いになる
- Kioskまたはモバイル注文でorganization境界の不一致が発生する
- ポイントやクーポンが二重付与・二重利用される
- Squareクーポンプールの参照が失敗する

ロールバック時は`MEMBERS_DATA_LAYOUT`を従来値へ戻し、members APIを`fr-agaruke`接続へ戻す。切替後に新Firestoreへ発生した書き込みは削除せず隔離し、旧側へ反映すべきイベントを監査してから再移行する。

## 未実行事項

この文書とコマンドの作成だけでは、本番データの読み取り、コピー、Firestore export、IAM、Secret Manager、Cloud Run設定変更は行われない。それぞれ実行前に対象と影響範囲を確認する。
