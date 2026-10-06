ARG GOLANG_IMAGE=golang:1.24-alpine
FROM ${GOLANG_IMAGE} AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} \
    HTTP_PROXY= \
    HTTPS_PROXY= \
    ALL_PROXY= \
    http_proxy= \
    https_proxy= \
    all_proxy= \
    NO_PROXY="*" \
    no_proxy="*" \
    GOWORK=off
WORKDIR /src
COPY . .
WORKDIR /src/cmd/network-provider
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/network-provider .

FROM alpine:3.20
RUN apk add --no-cache helm ca-certificates
COPY --from=build /out/network-provider /usr/local/bin/network-provider
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["network-provider"]
