# syntax=docker/dockerfile:1

# Build stage runs on the build machine's native platform and cross-compiles
# for the target platform, so no QEMU emulation is needed for the Go compile.
FROM --platform=$BUILDPLATFORM golang:1.26 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go test ./...

# GOARM takes only the digit (TARGETVARIANT is v5, v6 or v7), so strip the "v".
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ "$TARGETARCH" = "arm" ]; then export GOARM="${TARGETVARIANT#v}"; fi; \
    mkdir -p /out && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    go build -trimpath -ldflags "-s -w" -o /out/ecowitt-statsd .

# Final stage has no ARCH prefix: buildx selects the target platform automatically.
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -S -g 10001 ecowitt && \
    adduser -S -u 10001 -G ecowitt ecowitt

WORKDIR /var/lib/ecowitt-statsd
RUN chown ecowitt:ecowitt /var/lib/ecowitt-statsd

COPY --from=builder /out/ecowitt-statsd /bin/ecowitt-statsd

USER 10001:10001

EXPOSE 9090

CMD ["/bin/ecowitt-statsd","-configFile=/var/lib/ecowitt-statsd/config.json"]
