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
WORKDIR /src/cmd/cilium-provider
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cilium-provider .
RUN curl -fsSL -o /tmp/helm.tgz https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz \
    && tar -xzf /tmp/helm.tgz -C /tmp linux-amd64/helm \
    && mv /tmp/linux-amd64/helm /out/helm
RUN mkdir -p /out/tmp/hnb-cilium-provider/.cache /out/tmp/hnb-cilium-provider/.config /out/tmp/hnb-cilium-provider/.data \
    && touch /out/tmp/hnb-cilium-provider/.keep

FROM scratch
COPY --from=build /out/cilium-provider /usr/local/bin/cilium-provider
COPY --from=build /out/helm /usr/local/bin/helm
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/tmp /tmp
USER 65532:65532
ENV HELM_PATH=/usr/local/bin/helm \
    KUBECONFIG_DIR=/tmp/hnb-cilium-provider \
    HELM_CACHE_HOME=/tmp/hnb-cilium-provider/.cache \
    HELM_CONFIG_HOME=/tmp/hnb-cilium-provider/.config \
    HELM_DATA_HOME=/tmp/hnb-cilium-provider/.data
ENTRYPOINT ["cilium-provider"]
