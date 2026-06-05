# SimpleChat Dockerfile - 多阶段构建

# 阶段1: 构建 Go 后端（内嵌前端）
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=1 go build -o simplechat .

# 阶段2: 运行镜像
FROM alpine:latest

RUN apk add --no-cache ca-certificates sqlite-libs tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /build/simplechat .

# 创建数据和配置目录
RUN mkdir -p /app/data /app/config

EXPOSE 8080

CMD ["./simplechat"]
