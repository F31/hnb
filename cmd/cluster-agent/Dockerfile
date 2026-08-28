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
# cluster-agent is its own module; build standalone (GOWORK=off) using the
# module's go.mod replace directives, copying only the packages it transitively
# requires (pkg/iam -> pkg/core, pkg/tunnel -> pkg/core).
COPY cmd/cluster-agent/ cmd/cluster-agent/
COPY pkg/core/ pkg/core/
COPY pkg/iam/ pkg/iam/
COPY pkg/tunnel/ pkg/tunnel/
WORKDIR /src/cmd/cluster-agent
RUN go mod download && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cluster-agent .

FROM scratch
COPY --from=build /out/cluster-agent /cluster-agent
USER 65532:65532
ENTRYPOINT ["/cluster-agent"]