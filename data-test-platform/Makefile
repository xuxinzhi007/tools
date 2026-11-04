.PHONY: all build run test clean

PORT ?= 9099
OPENAPI ?= ../data-service/openapi.yaml
LOG ?= ../data-service/logs/app.log
TARGET ?= 
SLOW_THRESHOLD ?= 200ms

all: build

# 编译生成单文件可执行程序
build:
	mkdir -p bin
	go build -ldflags "-s -w" -o bin/data-test-platform .
	@echo "✅ Build completed: bin/data-test-platform"

# 本地直接运行
run:
	go run main.go -port $(PORT) -openapi $(OPENAPI) -log $(LOG) -target "$(TARGET)" -slow-threshold $(SLOW_THRESHOLD)

# 单元测试
test:
	go test -v ./...

# 清理构建产物
clean:
	rm -rf bin/
