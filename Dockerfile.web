# syntax=docker/dockerfile:1
FROM node:22.13.1-bookworm-slim AS deps
WORKDIR /app/web
ENV NEXT_TELEMETRY_DISABLED=1
COPY web/package.json web/package-lock.json ./
RUN npm ci

FROM node:22.13.1-bookworm-slim AS build
WORKDIR /app/web
ARG CODELOCAL_BACKEND_URL=http://127.0.0.1:3333
ENV NEXT_TELEMETRY_DISABLED=1
ENV CODELOCAL_BACKEND_URL=$CODELOCAL_BACKEND_URL
COPY --from=deps /app/web/node_modules ./node_modules
COPY web/ ./
RUN npm run build

FROM node:22.13.1-bookworm-slim AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
COPY --from=build --chown=node:node /app/web/public ./public
COPY --from=build --chown=node:node /app/web/.next/standalone ./
COPY --from=build --chown=node:node /app/web/.next/static ./.next/static
USER node
EXPOSE 3000
CMD ["node", "server.js"]
