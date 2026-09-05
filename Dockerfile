# ---- 构建阶段 ----
FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY frontend/package*.json ./
RUN npm install --production=false
COPY frontend/ .
RUN npm run build

# ---- Go 编译阶段 ----
# 版本须与 backend/go.mod 的 go 指令一致（1.25）；低版本镜像会触发运行时工具链下载
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
# 将前端 dist 复制到后端 web/dist：运行时从磁盘读取（非 go:embed），须随二进制同目录部署
COPY --from=frontend-builder /app/dist ./web/dist
COPY backend/ .
ENV CGO_ENABLED=0
RUN go build -ldflags="-s -w" -o /out/server ./cmd/server

# ---- 运行时 ----
FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates
WORKDIR /app
COPY --from=builder /out/server .
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/healthz || exit 1
ENTRYPOINT ["/app/server"]
