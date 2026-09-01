FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/asku-api ./cmd/api

FROM postgres:17-alpine
USER nobody
COPY --from=build /out/asku-api /usr/local/bin/asku-api
EXPOSE 18080
ENTRYPOINT ["/usr/local/bin/asku-api"]
