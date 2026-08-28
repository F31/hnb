FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.work go.work.sum ./
COPY cmd/calico-provider/ cmd/calico-provider/
COPY pkg/network/ pkg/network/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/calico-provider ./cmd/calico-provider

FROM scratch
COPY --from=build /out/calico-provider /calico-provider
USER 65532:65532
ENTRYPOINT ["/calico-provider"]