FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} GOWORK=off
WORKDIR /src
COPY . .
WORKDIR /src/cmd/app-market
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app-market .
RUN mkdir -p /out/artifact-transfer

FROM scratch
COPY --from=build /out/app-market /app-market
COPY --from=build --chown=65532:65532 /out/artifact-transfer /var/lib/hnb/artifact-transfer
USER 65532:65532
ENTRYPOINT ["/app-market"]
