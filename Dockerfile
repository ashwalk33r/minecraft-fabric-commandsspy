# Declared before the first FROM so it is usable in the second FROM line.
ARG JAVA_VERSION=21
ARG BASE_VARIANT=jammy

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY tools/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tools ./

# Stage 2: the server image. curl fetches the Fabric launcher at runtime.
FROM eclipse-temurin:${JAVA_VERSION}-jre-${BASE_VARIANT}
ARG BASE_VARIANT=jammy

# eclipse-temurin:<N>-jre-alpine measured ~10-36% faster `docker build` and
# ~25-30% smaller than -jre-jammy (real docker build/pull measurements,
# 2026-08-18), and Adoptium supports alpine as a first-class variant. Only
# floors 21/25/26 use it: those are the only floors with an arm64 alpine
# tag (8/11/17's alpine tags are amd64-only, which would force local arm64
# dev under qemu with no matching build-time win). bash is required on the
# alpine path: e2e-entrypoint.sh has a `#!/bin/bash` shebang and the alpine
# Temurin JRE image ships no bash by default (confirmed via a real boot
# test: without it, the container fails immediately with "exec ...: no
# such file or directory").
RUN if [ "$BASE_VARIANT" = "alpine" ]; then \
      apk add --no-cache curl bash; \
    else \
      apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*; \
    fi

COPY --from=build /tools /usr/local/bin/tools

WORKDIR /fabric-server

COPY scripts/e2e-entrypoint.sh /fabric-server/entrypoint.sh
RUN chmod +x /fabric-server/entrypoint.sh

ENTRYPOINT ["/fabric-server/entrypoint.sh"]
