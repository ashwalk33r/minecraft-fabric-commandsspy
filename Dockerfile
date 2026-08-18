# Declared before the first FROM so it is usable in the second FROM line.
ARG JAVA_VERSION=21

# Stage 1: build the harness tools binary (bot/rcon clients) — static, ~3MB.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY tools/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tools ./

# Stage 2: the server image. curl fetches the Fabric launcher at runtime.
FROM eclipse-temurin:${JAVA_VERSION}-jre-jammy

RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*

COPY --from=build /tools /usr/local/bin/tools

WORKDIR /fabric-server

COPY scripts/e2e-entrypoint.sh /fabric-server/entrypoint.sh
RUN chmod +x /fabric-server/entrypoint.sh

ENTRYPOINT ["/fabric-server/entrypoint.sh"]
