DB_DATABASE := foodrecords
DB_USER := user
DB_PASSWORD := password

# ローカル開発環境にdocker-composeサービス群を起動する
.PHONY: up
up:
	cd etc/docker; docker-compose -f docker-compose_prod.yml up -d

# ローカル開発環境にdocker-composeサービス群をビルドして起動する
.PHONY: upbuild
upbuild:
	cd etc/docker; docker-compose -f docker-compose_prod.yml up -d --build

# ローカル開発環境のdocker-composeサービス群を停止する
.PHONY: down
down:
	cd etc/docker; docker-compose -f docker-compose_prod.yml down

# ローカルのMySQLコンテナに接続する
.PHONY: mysql
mysql:
	cd etc/docker; docker-compose -f docker-compose_dev.yml exec mysql unifire-mysql -u${DB_USER} -p${DB_PASSWORD} ${DB_DATABASE}

# docker-composeサービス群から出力されるログを表示する
.PHONY: logs
logs:
	cd etc/docker; docker-compose -f docker-compose_dev.yml logs -f

# コード生成する
.PHONY: gen
gen:
	XDG_CONFIG_HOME=etc sqlboiler -f docker-compose_dev.yml mysql

# 開発用
.PHONY: ud
ud:
	cd etc/docker; docker-compose -f docker-compose_dev.yml up -d --build

.PHONY: dd
dd:
	cd etc/docker; docker-compose -f docker-compose_dev.yml down


# ローカルdevサーバーを起動する（Firestore・LINE不要）
.PHONY: dev
dev:
	go run ./cmd/devserver/

# orderecのFirebase Emulatorへ接続する会員API。外部LINEは呼ばず、
# local-line:<user>形式の開発トークンだけを追加で許可する。
.PHONY: emulator-dev
emulator-dev:
	ENV=local PROJECT_ID=demo-orderec \
	FIRESTORE_EMULATOR_HOST=127.0.0.1:8085 \
	ORGANIZATION_UUID=35095fe0-1efc-40ff-bd13-9720c6d09e0f \
	MEMBERS_DATA_LAYOUT=organization \
	MEMBERS_SERVICE_KEY=local-members-service-key \
	go run ./cmd/api

.PHONY: deploy
deploy:
	cd etc/docker; docker-compose -f docker-compose_prod.yml up -d --build \
	&& docker push gcr.io/fr-agaruke/members-api \
	&& gcloud run deploy members-api \
	  --platform managed \
	  --image gcr.io/fr-agaruke/members-api \
	  --region us-central1 \
	  --project fr-agaruke \
	  --update-secrets="DISCORD_WEBHOOK_URL=discord-webhook-url:latest" \
	  --update-env-vars="SQUARE_POOL_ALERT_THRESHOLD=20"
