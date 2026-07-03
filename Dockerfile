FROM golang:1.25-alpine AS build

WORKDIR /src
COPY gmr-engine/go.mod gmr-engine/go.sum ./
RUN go mod download

COPY gmr-engine ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gmr-engine ./cmd/api

FROM oven/bun:1.3.13

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/*

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile --production

COPY gmr-engine/scripts ./scripts
COPY --from=build /out/gmr-engine /usr/local/bin/gmr-engine
RUN command -v bun \
    && command -v wget \
    && test -f /app/scripts/erc20-console.ts

EXPOSE 8090
CMD ["gmr-engine"]
