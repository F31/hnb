FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gslb-controller .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/gslb-controller /usr/local/bin/gslb-controller
EXPOSE 8080
ENTRYPOINT ["gslb-controller"]