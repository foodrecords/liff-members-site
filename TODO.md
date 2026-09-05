# agaruke-members TODO

## 会員登録同意

- [x] モバイルオーダー用`POST /members/register`で規約・プライバシーポリシー同意を保存し、通常GETによる未登録会員の自動作成を停止する
- [x] members-siteの新規会員導線も既存`terms.html`確認後に`POST /members/register`を呼ぶ方式へ変更してからmembers APIを本番反映する（2026-09-05、規約・プライバシーポリシーへの明示同意UIを追加し、members API revision `members-api-00037-9dd`を本番反映）

## 本番Firestore統合

- [ ] 本番会員カードの`fr-agaruke`とモバイルオーダーの`food-records-prod`を統合する移行方針を確定する
- [x] `fr-agaruke`のmembers、member_numbers、coupons、point_logs、reward_catalog、serials、Squareクーポンプール、Kiosk会員連携データの全件数と参照関係を棚卸しする（2026-09-05、collection groupパス正規化修正後に永続データ320文書、会員クーポン21件、ポイント履歴13件、QR利用履歴1件を確認）
- [x] 移行先を`food-records-prod/organizations/35095fe0-1efc-40ff-bd13-9720c6d09e0f/...`として、読み取り専用の事前検査と再実行可能な本番マイグレーションを実装し、本番コピー・独立verifyを完了する（2026-09-05、漏れていたサブコレクション35件をmissing-onlyで追加。クーポン21件は欠損・余分・内容不一致0）
- [x] LINE User ID、会員番号、ポイント残高、累積ポイント、ランク、クーポン状態、有効期限、冪等付与履歴が移行前後で一致する検証レポートを出力する（2026-09-05、最新285文書を再コピーし、独立verifyで欠損・余分・内容不一致0。レポートはGit管理外に保存）
- [ ] 移行中の更新を取りこぼさない差分同期または短時間の書込停止手順を決定する
- [/] members APIとモバイルオーダーAPIを`food-records-prod`のorganization配下へ切り替える手順、Secret Manager、IAM、Firestore index・TTLを整備する（2026-09-05、members APIのIAM・接続先・organization layout切替まで完了。モバイルオーダーAPI、index・TTLの最終確認は未実施）
- [ ] 切替後に会員カード、Kiosk連携、ポイント付与、クーポン利用、モバイル注文、注文履歴、LINE通知を本番相当環境で結合試験する
- [/] 問題発生時に`fr-agaruke`へ戻せるロールバック条件・手順・切替前バックアップを準備する（手順・判定条件を文書化し、2026-08-28に30日保持の本番export取得済み。復元リハーサル待ち）
- [/] 検証完了後にのみ`MEMBERS_DATA_LAYOUT=organization`を本番で有効化し、旧プロジェクトの停止・保管時期を決定する（2026-09-05、members APIで有効化済み。旧プロジェクトはロールバック用に保持し、停止時期は未決定）
