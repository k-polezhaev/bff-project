FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY go.sum .

RUN go mod download
COPY main.go .
RUN go build -ldflags "-s -w" -o app main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/app .
COPY --from=builder /app/go.mod . 
EXPOSE 8080

CMD ["./app"]