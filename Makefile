# SimpleChat Makefile

FRONTEND_DIR = backend/frontend
LIBS_DIR     = $(FRONTEND_DIR)/libs
BINARY       = simplechat
GO_DIR       = backend

MARKED_VER   = 15.0.4
HLJS_VER     = 11.11.1
DOMPURIFY_VER = 3.2.4

.PHONY: all deps build run docker clean

all: deps build

# 下载前端依赖
deps: $(LIBS_DIR)/marked.min.js $(LIBS_DIR)/highlight.min.js $(LIBS_DIR)/github-dark.min.css $(LIBS_DIR)/purify.min.js

$(LIBS_DIR):
	mkdir -p $(LIBS_DIR)

$(LIBS_DIR)/marked.min.js: | $(LIBS_DIR)
	@echo "Downloading marked.js $(MARKED_VER)..."
	curl -fsSL -o $@ https://cdn.jsdelivr.net/npm/marked@$(MARKED_VER)/marked.min.js

$(LIBS_DIR)/highlight.min.js: | $(LIBS_DIR)
	@echo "Downloading highlight.js $(HLJS_VER)..."
	curl -fsSL -o $@ https://cdn.jsdelivr.net/npm/@highlightjs/cdn-assets@$(HLJS_VER)/highlight.min.js

$(LIBS_DIR)/github-dark.min.css: | $(LIBS_DIR)
	@echo "Downloading github-dark theme..."
	curl -fsSL -o $@ https://cdn.jsdelivr.net/npm/@highlightjs/cdn-assets@$(HLJS_VER)/styles/github-dark.min.css

$(LIBS_DIR)/purify.min.js: | $(LIBS_DIR)
	@echo "Downloading DOMPurify $(DOMPURIFY_VER)..."
	curl -fsSL -o $@ https://cdn.jsdelivr.net/npm/dompurify@$(DOMPURIFY_VER)/dist/purify.min.js

# 编译 Go 后端
build: deps
	@echo "Building $(BINARY)..."
	cd $(GO_DIR) && go build -o ../$(BINARY) .

# 编译并运行
run: build
	./$(BINARY)

# Docker 构建
docker:
	docker build -t simplechat .

# Docker Compose 启动
up:
	docker compose up -d

# Docker Compose 停止
down:
	docker compose down

# 清理产物
clean:
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(LIBS_DIR)
	@echo "Cleaned."
