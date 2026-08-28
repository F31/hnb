FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.work go.work.sum ./
COPY cmd/kube-ovn-provider/ cmd/kube-ovn-provider/
COPY pkg/network/ pkg/network/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kube-ovn-provider ./cmd/kube-ovn-provider

FROM scratch
COPY --from=build /out/kube-ovn-provider /kube-ovn-provider
USER 65532:65532
ENTRYPOINT ["/kube-ovn-provider"]