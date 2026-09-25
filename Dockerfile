ARG GO_VERSION=1.24.0
FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run ./cmd/docs-generator \
    && cp docs/api.html internal/handlers/docs.html \
    && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-buildid=" -o /out/counter .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && adduser -D -H -u 10001 counter
COPY --from=build /out/counter /usr/local/bin/counter
USER counter
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/counter"]
