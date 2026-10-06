ARG GOLANG_IMAGE=golang:1.24
FROM ${GOLANG_IMAGE} AS build
ARG GOPROXY=https://goproxy.cn,direct
# 本模块 go.mod 含指向 ../../pkg/gslb 的 replace，需以仓库根为构建上下文（与 apiserver 一致）。
# 用 scratch 而非 alpine： alpine 的 apk 在受限网络下不可用，二进制为静态编译无需 CA 包。
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
WORKDIR /src/cmd/gslb-controller
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gslb-controller .

FROM scratch
COPY --from=build /out/gslb-controller /gslb-controller
EXPOSE 8080
ENTRYPOINT ["/gslb-controller"]
