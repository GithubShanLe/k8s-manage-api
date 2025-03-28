FROM golang:1.19 as builder

WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o k8s-manage-api

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/k8s-manage-api .
EXPOSE 8080
CMD ["./k8s-manage-api"]