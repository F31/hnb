FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.work go.work.sum ./
COPY cmd/network-registry/ cmd/network-registry/
COPY pkg/network/ pkg/network/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/network-registry ./cmd/network-registry

FROM scratch
COPY --from=build /out/network-registry /network-registry
USER 65532:65532
ENTRYPOINT ["/network-registry"]