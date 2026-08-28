FROM golang:1.24 AS build
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/rbac-syncer .

FROM scratch
COPY --from=build /out/rbac-syncer /rbac-syncer
USER 65532:65532
EXPOSE 8080
EXPOSE 8081
ENTRYPOINT ["/rbac-syncer"]