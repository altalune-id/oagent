# syntax=docker/dockerfile:1.10

FROM node:24.22.1-alpine AS build
WORKDIR /src
ENV PNPM_HOME=/pnpm \
    PATH=/pnpm:$PATH \
    HUSKY=0 \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/runtime/package.json apps/runtime/
COPY ui/vitest-config/package.json ui/vitest-config/
RUN corepack enable \
    && pnpm --version
RUN --mount=type=cache,id=pnpm,target=/pnpm/store \
    pnpm install --frozen-lockfile --ignore-scripts --filter @oagent/runtime...

COPY apps/runtime apps/runtime
RUN pnpm --filter @oagent/runtime build \
    && pnpm --filter @oagent/runtime deploy --prod --legacy /out

FROM node:24.22.1-alpine
WORKDIR /app
ENV NODE_ENV=production \
    PORT=5160

COPY --from=build --chown=1000:1000 /out/package.json ./package.json
COPY --from=build --chown=1000:1000 /out/node_modules ./node_modules
COPY --from=build --chown=1000:1000 /src/apps/runtime/dist ./dist

USER 1000:1000
EXPOSE 5160

CMD ["node", "dist/main.js"]
