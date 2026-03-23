FROM golang:1.21-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod ./
COPY . .

RUN go build -o /out/tecora-api ./cmd/api

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/tecora-api /app/tecora-api

EXPOSE 8080

ENTRYPOINT ["/app/tecora-api"]
