FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /network-provider .

FROM alpine:3.20

RUN apk add --no-cache helm ca-certificates

COPY --from=builder /network-provider /usr/local/bin/network-provider

EXPOSE 8080

ENTRYPOINT ["network-provider"]