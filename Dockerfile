# syntax=docker/dockerfile:1

#
# Build the cluster-specific Swarm entrypoint.
#
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS entrypoint-builder

WORKDIR /src

COPY go.mod ./
COPY cmd/swarm-entrypoint ./cmd/swarm-entrypoint

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/swarm-entrypoint \
        ./cmd/swarm-entrypoint


#
# Download the official fbs-interlock-gateway release for
# the target container architecture.
#
FROM --platform=$BUILDPLATFORM alpine:3.22 AS gateway

RUN apk add --no-cache \
    ca-certificates \
    curl

WORKDIR /out

ARG TARGETOS
ARG TARGETARCH
ARG GATEWAY_VERSION=v4.0.1

RUN set -eux; \
    test "${TARGETOS}" = "linux"; \
    case "${TARGETARCH}" in \
        amd64|arm64) ;; \
        *) \
            echo "Unsupported architecture: ${TARGETARCH}" >&2; \
            exit 1; \
            ;; \
    esac; \
    asset="fbs-interlock-gateway-linux-${TARGETARCH}"; \
    base_url="https://github.com/williamveith/fbs-interlock-gateway/releases/download/${GATEWAY_VERSION}"; \
    curl --fail --location --show-error \
        --output "${asset}" \
        "${base_url}/${asset}"; \
    curl --fail --location --show-error \
        --output "${asset}.sha256" \
        "${base_url}/${asset}.sha256"; \
    sha256sum -c "${asset}.sha256"; \
    chmod 0755 "${asset}"; \
    mv "${asset}" /out/fbs-interlock-gateway


#
# Final cluster image.
#
FROM litestream/litestream:0.5.17-scratch

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
ARG GATEWAY_VERSION=v4.0.1

LABEL org.opencontainers.image.title="fbs-interlock-gateway-cluster" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${DATE}" \
      org.opencontainers.image.source="https://github.com/williamveith/fbs-interlock-gateway-cluster" \
      org.opencontainers.image.description="Docker Swarm image for FBS Interlock Gateway with Litestream, Cloudflare R2 persistence, and multi-architecture Linux support." \
      io.fbs-interlock-gateway.version="${GATEWAY_VERSION}"

COPY --from=gateway \
    /out/fbs-interlock-gateway \
    /fbs-interlock-gateway

COPY --from=entrypoint-builder \
    /out/swarm-entrypoint \
    /swarm-entrypoint

COPY litestream.yml /etc/litestream.yml

ENTRYPOINT ["/swarm-entrypoint"]
CMD ["replicate"]