FROM alpine:3.20

ARG BUILD_VERSION=unknown

LABEL version="${BUILD_VERSION}"

RUN apk add --no-cache curl

COPY . /app
WORKDIR /app

CMD ["echo", "api-gateway running"]
