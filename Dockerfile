FROM --platform=$BUILDPLATFORM golang:1.21.1-alpine3.18 as builder

RUN apk add --no-cache make ca-certificates gcc musl-dev linux-headers git jq bash

COPY ./go.mod /app/go.mod
COPY ./go.sum /app/go.sum

WORKDIR /app

RUN go mod download

COPY ./bindings    /app/bindings
COPY ./core        /app/core
COPY ./cmd         /app/cmd
COPY ./Makefile    /app/Makefile
COPY ./bot.mainnet.toml    /app/bot.mainnet.toml

WORKDIR /app/

RUN make build-go

FROM alpine:3.18

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/bot              /usr/local/bin
COPY --from=builder /app/bot.mainnet.toml /app/bot.mainnet.toml

WORKDIR /app

ENTRYPOINT ["/usr/local/bin/bot"]
