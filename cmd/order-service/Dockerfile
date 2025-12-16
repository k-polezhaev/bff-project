FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o app main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/app .
CMD ["./app"]