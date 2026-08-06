# CGO is required: the event store uses mattn/go-sqlite3.
#
# Nothing is cross-compiled here on purpose. buildx runs this stage under the
# target platform, so on a matching runner the Go toolchain and the C compiler
# are both native and no cross toolchain has to be wired up.
FROM golang:1.25-alpine AS build
RUN apk add --no-cache gcc musl-dev
WORKDIR /src

# Dependencies first: they change far less often than the source, so this layer
# survives most rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# -trimpath keeps build paths out of the binary, so the same source gives the
# same bytes wherever it was built.
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/nostrel ./cmd/nostrel

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 nostrel
COPY --from=build /out/nostrel /usr/local/bin/nostrel
USER nostrel
WORKDIR /data
ENV DB_PATH=/data/nostrel.db RELAY_PORT=3334
EXPOSE 3334
VOLUME /data
ENTRYPOINT ["nostrel"]

# Links the image back to this repository on the GHCR package page.
LABEL org.opencontainers.image.source="https://github.com/PIXELZX0/nostrel" \
      org.opencontainers.image.description="라이트닝 결제로 쓰기 화이트리스트를 운영하는 Nostr 릴레이" \
      org.opencontainers.image.licenses="MIT"
