ARG GOLANG_IMAGE=golang:1.24
ARG HELM_VERSION=v3.16.2
FROM ${GOLANG_IMAGE} AS build
ARG GOPROXY=https://goproxy.cn,direct
ARG HELM_VERSION
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
WORKDIR /src/cmd/plugin-worker
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/plugin-worker .
RUN curl -fsSL -o /tmp/helm.tgz https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz \
    && tar -xzf /tmp/helm.tgz -C /tmp linux-amd64/helm \
    && mv /tmp/linux-amd64/helm /out/helm

FROM scratch
COPY --from=build /out/plugin-worker /plugin-worker
COPY --from=build /out/helm /usr/local/bin/helm
USER 65532:65532
ENV HELM_PATH=/usr/local/bin/helm KUBECONFIG_DIR=/tmp/hnb-plugin-worker
EXPOSE 8080
ENTRYPOINT ["/plugin-worker"]