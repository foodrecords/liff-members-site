# agaruke-members TODO

## 会員登録同意

- [x] モバイルオーダー用`POST /members/register`で規約・プライバシーポリシー同意を保存し、通常GETによる未登録会員の自動作成を停止する
- [ ] members-siteの新規会員導線も既存`terms.html`確認後に`POST /members/register`を呼ぶ方式へ変更してからmembers APIを本番反映する

## 本番Firestore統合

- [ ] 本番会員カードの`fr-agaruke`とモバイルオーダーの`food-records-prod`を統合する移行方針を確定する
- [x] `fr-agaruke`のmembers、member_numbers、coupons、point_logs、reward_catalog、serials、Squareクーポンプール、Kiosk会員連携データの全件数と参照関係を棚卸しする（2026-08-28、読み取り専用inventory実施。永続データ281文書、会員番号重複index 9件、有効Kiosk token 1件を確認）
- [/] 移行先を`food-records-prod/organizations/35095fe0-1efc-40ff-bd13-9720c6d09e0f/...`として、読み取り専用の事前検査と再実行可能な本番マイグレーションを実装する（コマンド・集計検証・安全停止を実装済み。本番棚卸し・リハーサル待ち）
- [ ] LINE User ID、会員番号、ポイント残高、累積ポイント、ランク、クーポン状態、有効期限、冪等付与履歴が移行前後で一致する検証レポートを出力する
- [ ] 移行中の更新を取りこぼさない差分同期または短時間の書込停止手順を決定する
- [ ] members APIとモバイルオーダーAPIを`food-records-prod`のorganization配下へ切り替える手順、Secret Manager、IAM、Firestore index・TTLを整備する
- [ ] 切替後に会員カード、Kiosk連携、ポイント付与、クーポン利用、モバイル注文、注文履歴、LINE通知を本番相当環境で結合試験する
- [/] 問題発生時に`fr-agaruke`へ戻せるロールバック条件・手順・切替前バックアップを準備する（手順・判定条件を文書化済み。本番export・復元確認待ち）
- [ ] 検証完了後にのみ`MEMBERS_DATA_LAYOUT=organization`を本番で有効化し、旧プロジェクトの停止・保管時期を決定する
