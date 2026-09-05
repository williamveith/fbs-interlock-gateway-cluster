# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
    -trimpath \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${COMMIT} \
      -X main.date=${DATE}" \
    -o /out/fbs-interlock-gateway-cluster \
    ./cmd/fbs-interlock-gateway-cluster

FROM scratch

COPY --from=builder \
    /out/fbs-interlock-gateway-cluster \
    /fbs-interlock-gateway-cluster

USER 65532:65532

ENTRYPOINT ["/fbs-interlock-gateway-cluster"]