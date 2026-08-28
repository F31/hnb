FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/operation-worker .

FROM scratch
COPY --from=build /out/operation-worker /operation-worker
USER 65532:65532
ENTRYPOINT ["/operation-worker"]