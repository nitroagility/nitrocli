FROM golang:1.22-alpine AS builder
ARG BUILD_VERSION=unknown
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -ldflags="-X main.version=${BUILD_VERSION}" -o /api-gateway .

FROM alpine:3.20
LABEL version="${BUILD_VERSION}"
COPY --from=builder /api-gateway /usr/local/bin/api-gateway
EXPOSE 8080
CMD ["api-gateway"]
