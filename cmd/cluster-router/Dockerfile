FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.work go.work.sum ./
COPY cmd/cluster-router/ cmd/cluster-router/
COPY pkg/core/ pkg/core/
COPY pkg/router/ pkg/router/
COPY pkg/tunnel/ pkg/tunnel/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cluster-router ./cmd/cluster-router

FROM scratch
COPY --from=build /out/cluster-router /cluster-router
USER 65532:65532
ENTRYPOINT ["/cluster-router"]