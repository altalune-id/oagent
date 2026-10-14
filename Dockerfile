# syntax=docker/dockerfile:1.10

FROM --platform=$BUILDPLATFORM node:24-alpine AS ui
WORKDIR /src
ENV PNPM_HOME=/pnpm \
    PATH=/pnpm:$PATH \
    HUSKY=0 \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY ui/console/package.json ui/console/
COPY ui/mcp/package.json ui/mcp/
COPY ui/web/package.json ui/web/
COPY ui/scripts/package.json ui/scripts/
COPY ui/vitest-config/package.json ui/vitest-config/
RUN corepack enable \
    && pnpm --version
RUN --mount=type=cache,id=pnpm,target=/pnpm/store \
    pnpm install --frozen-lockfile --ignore-scripts

RUN apk add --no-cache make
COPY Makefile ./
COPY version version
COPY ui ui
COPY scope scope
COPY apperror apperror
COPY password password
COPY internal internal
RUN make ui

FROM --platform=$BUILDPLATFORM golang:1.26.9-alpine AS build
WORKDIR /src
RUN apk add --no-cache make

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
# make ui placed the builds where the binary embeds them; carry them over from the ui stage
COPY --from=ui /src/internal/web/static/ internal/web/static/
COPY --from=ui /src/internal/web/console/public/ internal/web/console/public/
COPY --from=ui /src/internal/mcp/ui/assets/ internal/mcp/ui/assets/

# VERSION defaults to version/VERSION; COMMIT and BUILD_TIME come from the build (no .git in the context)
ARG VERSION=
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETOS
ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" make build ${VERSION:+VERSION=$VERSION} COMMIT="${COMMIT}" BUILD="${BUILD_TIME}" GOTOOLCHAIN=local

FROM gcr.io/distroless/static:nonroot AS runtime

COPY --from=build /src/bin/oagent /oagent

USER 65532:65532
EXPOSE 5150

ENTRYPOINT ["/oagent"]
CMD ["serve"]
