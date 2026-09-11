# syntax=docker/dockerfile:1
FROM node:22-bookworm-slim AS deps
WORKDIR /app
COPY web/apps/pool/package.json web/apps/pool/package-lock.json ./
RUN npm ci

FROM node:22-bookworm-slim AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY web/src/lib/i18n ./src/lib/i18n
WORKDIR /app/apps/pool
COPY web/apps/pool ./
RUN npm run build

FROM node:22-bookworm-slim AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
COPY --from=build /app/apps/pool/.next/standalone ./
COPY --from=build /app/apps/pool/.next/static ./apps/pool/.next/static
USER node
EXPOSE 3000
CMD ["node", "apps/pool/server.js"]
