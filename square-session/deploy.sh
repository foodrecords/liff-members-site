#!/bin/bash
set -e

PROJECT_ID="fr-agaruke"
REGION="us-central1"
IMAGE="gcr.io/${PROJECT_ID}/square-session"
BUCKET_NAME="fr-agaruke-square-session"
SERVICE_ACCOUNT="square-session@${PROJECT_ID}.iam.gserviceaccount.com"
MEMBERS_API_URL="https://members-api-438247672947.us-central1.run.app"

echo "=== 1. GCS バケット作成（初回のみ）==="
gsutil mb -p ${PROJECT_ID} -l ${REGION} gs://${BUCKET_NAME} 2>/dev/null || echo "バケットは既に存在します"

echo "=== 2. サービスアカウント作成・権限付与（初回のみ）==="
gcloud iam service-accounts create square-session \
  --display-name="Square Session Manager" \
  --project=${PROJECT_ID} 2>/dev/null || echo "サービスアカウントは既に存在します"

# GCS への書き込み権限
gsutil iam ch serviceAccount:${SERVICE_ACCOUNT}:roles/storage.objectAdmin gs://${BUCKET_NAME}

# Firestore への書き込み権限（fill-pool が使用）
gcloud projects add-iam-policy-binding ${PROJECT_ID} \
  --member="serviceAccount:${SERVICE_ACCOUNT}" \
  --role="roles/datastore.user" 2>/dev/null || true

echo "=== 3. Secret Manager へ SQUARE_API_TOKEN を登録（初回のみ）==="
echo "※ SQUARE_API_TOKEN 環境変数を設定してから実行してください"
if [ -n "${SQUARE_API_TOKEN}" ]; then
  echo -n "${SQUARE_API_TOKEN}" | gcloud secrets create square-api-token \
    --data-file=- --project=${PROJECT_ID} 2>/dev/null || \
  echo -n "${SQUARE_API_TOKEN}" | gcloud secrets versions add square-api-token \
    --data-file=- --project=${PROJECT_ID}
else
  echo "  SQUARE_API_TOKEN 未設定 → スキップ（手動で登録してください）"
fi

echo "=== 4. Docker イメージビルド & プッシュ ==="
docker build --platform linux/amd64 -t ${IMAGE} .
docker push ${IMAGE}

echo "=== 5. Cloud Run Job: square-login（自動ログイン）==="
gcloud run jobs deploy square-login \
  --image=${IMAGE} \
  --region=${REGION} \
  --project=${PROJECT_ID} \
  --service-account=${SERVICE_ACCOUNT} \
  --memory=2Gi \
  --cpu=1 \
  --task-timeout=300 \
  --max-retries=1 \
  --command="node" \
  --args="dist/main.js" \
  --set-env-vars="GCS_BUCKET=${BUCKET_NAME}" \
  --set-secrets="SQUARE_EMAIL=square-email:latest,SQUARE_PASSWORD=square-password:latest,DISCORD_WEBHOOK_URL=discord-webhook-url:latest"

echo "=== 6. Cloud Run Job: fill-pool（プール補充）==="
# pricing_rule_id は Firestore reward_catalog から自動取得するため env vars での指定不要
gcloud run jobs deploy fill-pool \
  --image=${IMAGE} \
  --region=${REGION} \
  --project=${PROJECT_ID} \
  --service-account=${SERVICE_ACCOUNT} \
  --memory=512Mi \
  --cpu=1 \
  --task-timeout=600 \
  --max-retries=0 \
  --command="node" \
  --args="dist/fill-pool.js" \
  --set-env-vars="GCS_BUCKET=${BUCKET_NAME},PROJECT_ID=${PROJECT_ID},FILL_COUNT=50,FILL_DELAY_MS=300" \
  --set-secrets="SQUARE_API_TOKEN=square-api-token:latest,DISCORD_WEBHOOK_URL=discord-webhook-url:latest"

echo "=== 7. Cloud Scheduler: square-login-daily（毎朝4時 JST）==="
gcloud scheduler jobs create http square-login-daily \
  --location=${REGION} \
  --project=${PROJECT_ID} \
  --schedule="0 19 * * *" \
  --uri="https://${REGION}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/${PROJECT_ID}/jobs/square-login:run" \
  --http-method=POST \
  --oauth-service-account-email=${SERVICE_ACCOUNT} \
  --time-zone="UTC" \
  2>/dev/null || \
gcloud scheduler jobs update http square-login-daily \
  --location=${REGION} \
  --project=${PROJECT_ID} \
  --schedule="0 19 * * *"

echo "=== 8. Cloud Scheduler: pool-monitor-daily（毎朝9時 JST）==="
# members-api の ADMIN_TOKEN を環境変数から取得（未設定時はスキップ）
if [ -n "${ADMIN_TOKEN}" ]; then
  gcloud scheduler jobs create http pool-monitor-daily \
    --location=${REGION} \
    --project=${PROJECT_ID} \
    --schedule="0 0 * * *" \
    --uri="${MEMBERS_API_URL}/admin/pool-monitor" \
    --http-method=GET \
    --headers="Authorization=Bearer ${ADMIN_TOKEN}" \
    --time-zone="Asia/Tokyo" \
    2>/dev/null || \
  gcloud scheduler jobs update http pool-monitor-daily \
    --location=${REGION} \
    --project=${PROJECT_ID} \
    --schedule="0 0 * * *" \
    --headers="Authorization=Bearer ${ADMIN_TOKEN}"
else
  echo "  ADMIN_TOKEN 未設定 → pool-monitor-daily スキップ"
  echo "  手動設定: ADMIN_TOKEN=xxx ./deploy.sh で再実行"
fi

echo ""
echo "=== デプロイ完了 ==="
echo ""
echo "【プール補充フロー】"
echo "  1. ログイン:  GCS_BUCKET=${BUCKET_NAME} npm run login:manual"
echo "  2. 補充実行:  gcloud run jobs execute fill-pool --region=${REGION} --project=${PROJECT_ID}"
echo "     ※ pricing_rule_id は Firestore reward_catalog から自動取得されます"
echo ""
echo "【手動実行】"
echo "  ログイン自動化: gcloud run jobs execute square-login --region=${REGION} --project=${PROJECT_ID}"
echo "  プール確認:     curl -H 'Authorization: Bearer \${ADMIN_TOKEN}' ${MEMBERS_API_URL}/admin/pool-monitor"
