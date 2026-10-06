ARG GOLANG_IMAGE=golang:1.24
FROM ${GOLANG_IMAGE} AS build
ARG GOPROXY=https://goproxy.cn,direct
# 本模块 go.mod 含指向 ../../pkg/* 的 replace，需以仓库根为构建上下文（与 apiserver 一致）。
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
WORKDIR /src/cmd/operation-worker
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/operation-worker .

FROM scratch
COPY --from=build /out/operation-worker /operation-worker
USER 65532:65532
ENTRYPOINT ["/operation-worker"]
