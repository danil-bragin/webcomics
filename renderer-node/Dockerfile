FROM node:20-bookworm-slim

# Remotion / Chromium runtime deps.
RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium fonts-noto-color-emoji libnss3 libatk1.0-0 libatk-bridge2.0-0 \
    libcups2 libxcomposite1 libxdamage1 libxrandr2 libgbm1 libxkbcommon0 \
    libpango-1.0-0 libpangocairo-1.0-0 libasound2 ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*

ENV REMOTION_BROWSER_EXECUTABLE=/usr/bin/chromium

WORKDIR /app
COPY package.json tsconfig.json remotion.config.ts ./
RUN npm install
COPY src ./src

CMD ["npm", "start"]
