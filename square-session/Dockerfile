# Playwright 公式イメージ（Chromium 同梱）
FROM mcr.microsoft.com/playwright:v1.48.0-noble

WORKDIR /app

COPY package.json ./
RUN PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install

COPY tsconfig.json ./
COPY src/ ./src/
RUN npm run build

CMD ["node", "dist/main.js"]
