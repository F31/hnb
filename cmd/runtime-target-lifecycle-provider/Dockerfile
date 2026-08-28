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
WORKDIR /src/cmd/runtime-target-lifecycle-provider
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/runtime-target-lifecycle-provider .

FROM scratch
COPY --from=build /out/runtime-target-lifecycle-provider /runtime-target-lifecycle-provider
USER 65532:65532
EXPOSE 18082
ENTRYPOINT ["/runtime-target-lifecycle-provider"]
