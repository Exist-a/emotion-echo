# 构建阶段
FROM golang:1.26-alpine AS builder

WORKDIR /app

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server

# 运行阶段
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# 从构建阶段复制编译好的二进制文件
COPY --from=builder /app/server .
COPY --from=builder /app/config.yaml .
COPY --from=builder /app/configs ./configs

# 创建上传目录
RUN mkdir -p /root/uploads

EXPOSE 8080

CMD ["./server"]