ARG ARCH=

FROM ${ARCH}golang:1.26 as builder

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY ./ ./

RUN go test ./...

RUN GOOS=linux CGO_ENABLED=0 go build

FROM ${ARCH}alpine:3.21

COPY --from=builder /app/ecowitt-statsd /bin/ecowitt-statsd

EXPOSE 9090

CMD ["/bin/ecowitt-statsd","-configFile=/var/lib/ecowitt-statsd/config.json"]
