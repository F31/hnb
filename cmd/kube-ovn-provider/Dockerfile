ARG GOLANG_IMAGE=golang:1.24
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
WORKDIR /src/cmd/kube-ovn-provider
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kube-ovn-provider .

FROM alpine:3.20
RUN apk add --no-cache helm ca-certificates
COPY --from=build /out/kube-ovn-provider /usr/local/bin/kube-ovn-provider
USER 65532:65532
ENTRYPOINT ["kube-ovn-provider"]
