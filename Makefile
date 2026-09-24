# codex-session-doctor 的常用构建目标

BIN := codex-session-doctor
PREFIX ?= $(HOME)/.local

.PHONY: all build test vet fmt check install uninstall clean

all: check

# build 生成当前平台的二进制
build:
	go build -o $(BIN) ./cmd/codex-session-doctor

# test 运行全部单元测试
test:
	go test ./...

# vet 运行静态检查
vet:
	go vet ./...

# fmt 格式化代码
fmt:
	gofmt -w ./cmd ./internal

# check 是提交前的完整检查
check: vet test

# install 把二进制装到 $(PREFIX)/bin
install: build
	mkdir -p $(PREFIX)/bin
	install -m 0755 $(BIN) $(PREFIX)/bin/$(BIN)
	@echo "已安装到 $(PREFIX)/bin/$(BIN)"

# uninstall 移除已安装的二进制
uninstall:
	rm -f $(PREFIX)/bin/$(BIN)

clean:
	rm -f $(BIN)
	rm -rf dist
