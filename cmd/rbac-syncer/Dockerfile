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
WORKDIR /src/cmd/rbac-syncer
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/rbac-syncer .

FROM scratch
COPY --from=build /out/rbac-syncer /rbac-syncer
USER 65532:65532
EXPOSE 8080
EXPOSE 8081
ENTRYPOINT ["/rbac-syncer"]
