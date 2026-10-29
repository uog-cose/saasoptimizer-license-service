# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/license-service ./cmd/license-service

FROM alpine:3.22

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
