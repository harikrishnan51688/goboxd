FROM debian:bookworm-slim AS nsjail-base

RUN apt-get update && apt-get install -y --no-install-recommends

COPY scripts/ /app/scripts/
RUN chmod +x /app/scripts/install.sh && /app/scripts/install.sh

FROM nsjail-base AS nsjail-server




WORKDIR /app

