FROM eclipse-temurin:21-jre-jammy

RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*

WORKDIR /fabric-server

COPY scripts/smoke-entrypoint.sh /fabric-server/entrypoint.sh
RUN chmod +x /fabric-server/entrypoint.sh

ENTRYPOINT ["/fabric-server/entrypoint.sh"]
