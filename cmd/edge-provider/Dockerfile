FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/edge-provider .

FROM scratch
COPY --from=build /out/edge-provider /edge-provider
USER 65532:65532
EXPOSE 18081
ENTRYPOINT ["/edge-provider"]