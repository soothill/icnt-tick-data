# Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/icnt-tick-data ./cmd/pipeline

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/icnt-tick-data /usr/local/bin/icnt-tick-data
ENTRYPOINT ["/usr/local/bin/icnt-tick-data"]
