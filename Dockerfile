# SimpleChat Dockerfile - 多阶段构建

# 阶段1: 构建 Go 后端（内嵌前端）
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev curl

WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
RUN mkdir -p frontend/libs && \
    curl -sL -o frontend/libs/marked.min.js https://cdn.jsdelivr.net/npm/marked@15.0.4/marked.min.js && \
    curl -sL -o frontend/libs/highlight.min.js https://cdn.jsdelivr.net/npm/@highlightjs/cdn-assets@11.11.1/highlight.min.js && \
    curl -sL -o frontend/libs/github-dark.min.css https://cdn.jsdelivr.net/npm/@highlightjs/cdn-assets@11.11.1/styles/github-dark.min.css && \
    curl -sL -o frontend/libs/purify.min.js https://cdn.jsdelivr.net/npm/dompurify@3.2.4/dist/purify.min.js

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
