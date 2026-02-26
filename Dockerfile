FROM golang:1.24-bookworm AS builder

RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /src
COPY app/ ./app/

WORKDIR /src/app
RUN templ generate
RUN CGO_ENABLED=0 go build -o /opswise ./cmd/

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ansible \
        openssh-client \
        git \
        ca-certificates \
        docker.io \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash opswise

WORKDIR /app

COPY --from=builder /opswise /app/opswise
COPY app/web/static /app/web/static
COPY deploy/ /app/deploy/

RUN mkdir -p /app/data && chown opswise:opswise /app/data

USER opswise

ENV OPSWISE_DB=/app/data/opswise.db
ENV OPSWISE_DEPLOY_DIR=/app/deploy

EXPOSE 8080

ENTRYPOINT ["/app/opswise"]
