FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/asku-admin ./cmd/server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S asku && adduser -S -G asku asku
COPY --from=build /out/asku-admin /usr/local/bin/asku-admin
USER asku
EXPOSE 18090
ENTRYPOINT ["/usr/local/bin/asku-admin"]
