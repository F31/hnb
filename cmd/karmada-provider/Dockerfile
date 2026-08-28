FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.work go.work.sum ./
COPY cmd/karmada-provider/ cmd/karmada-provider/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/karmada-provider ./cmd/karmada-provider

FROM scratch
COPY --from=build /out/karmada-provider /karmada-provider
USER 65532:65532
ENTRYPOINT ["/karmada-provider"]