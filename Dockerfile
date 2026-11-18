# syntax=docker/dockerfile:1

ARG SAASOPT_GO_TOOLCHAIN=1.26-alpine
ARG SAASOPT_VERSION=dev
ARG SAASOPT_REVISION=unknown
ARG SAASOPT_BUILD_DATE=unknown

FROM golang:${SAASOPT_GO_TOOLCHAIN} AS build

WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/license-service ./cmd/license-service

FROM alpine:3.22

ARG SAASOPT_GO_TOOLCHAIN
ARG SAASOPT_VERSION
ARG SAASOPT_REVISION
ARG SAASOPT_BUILD_DATE

LABEL org.opencontainers.image.title="SaasOptimizer License Service" \
    org.opencontainers.image.description="Non-production License Service for development and CI" \
    org.opencontainers.image.source="https://github.com/uog-cose/saasoptimizer-license-service" \
    org.opencontainers.image.version="$SAASOPT_VERSION" \
    org.opencontainers.image.revision="$SAASOPT_REVISION" \
    org.opencontainers.image.created="$SAASOPT_BUILD_DATE" \
    com.saasoptimizer.build.go-toolchain="$SAASOPT_GO_TOOLCHAIN" \
    com.saasoptimizer.environment="non-production"

RUN addgroup -S saasoptimizer && adduser -S -G saasoptimizer saasoptimizer
WORKDIR /app
COPY --from=build /out/license-service /app/license-service
USER saasoptimizer

EXPOSE 55154
ENV LICENSE_SERVICE_ADDR=:55154 \
    LICENSE_SERVICE_REQUIRE_AUTH=true \
    LICENSE_SERVICE_SCENARIO=allow

HEALTHCHECK --interval=5s --timeout=3s --start-period=2s --retries=12 \
    CMD wget -q -O - http://127.0.0.1:55154/v1/health >/dev/null || exit 1

ENTRYPOINT ["/app/license-service"]
