BINARY := sql-monitor
PKG    := ./cmd/sql-monitor
LOG    ?= /tmp/data-service-local.log
CONFIG ?= sql-monitor.local.json

# env（或 ENV，大小写均可）选择数据源：
#   空 / local    本地日志 $(LOG)
#   k8s           集群全部 data-service pod（忽略 lane）
#   dev/dev1/dev2/dev3/test  集群日志（指定该 lane）
ENV ?=
ifeq ($(strip $(ENV)),)
ENV := $(strip $(env))
endif

# 集群连接参数（可用 make ... K8S_KUBECONFIG=... 覆盖；kubeconfig 支持 ~ / $HOME）
K8S_KUBECONFIG ?= $$HOME/Downloads/ns-cloud-dev.readonly.yml
K8S_CONTEXT    ?= dev-user-context
K8S_NAMESPACE  ?= ns-cloud-dev
K8S_APP        ?= data-service

.DEFAULT_GOAL := run

.PHONY: run build demo init test test-demo vet clean help

run: build
	@case "$(ENV)" in \
	  ""|local) \
	    if [ -f "$(CONFIG)" ]; then \
	      echo ">> 本地日志 $(LOG) + 配置 $(CONFIG)"; \
	      ./$(BINARY) --config "$(CONFIG)" --log "$(LOG)"; \
	    else \
	      echo ">> 本地日志 $(LOG)（无配置，飞书关闭）"; \
	      ./$(BINARY) --log "$(LOG)"; \
	    fi ;; \
	  k8s|dev|dev1|dev2|dev3|test) \
	    cfg=""; \
	    if [ -f "$(CONFIG)" ]; then cfg="--config $(CONFIG)"; else echo "（无配置，飞书关闭）"; fi; \
	    if [ "$(ENV)" = "k8s" ]; then \
	      echo ">> 集群全部 $(K8S_APP) pod（$(K8S_NAMESPACE)）"; \
	      ./$(BINARY) $$cfg --k8s-all \
	        --k8s-kubeconfig $(K8S_KUBECONFIG) --k8s-context "$(K8S_CONTEXT)" \
	        --k8s-namespace "$(K8S_NAMESPACE)" --k8s-app "$(K8S_APP)"; \
	    else \
	      echo ">> 集群日志 lane=$(ENV)（$(K8S_NAMESPACE)）"; \
	      ./$(BINARY) $$cfg --k8s-lane "$(ENV)" \
	        --k8s-kubeconfig $(K8S_KUBECONFIG) --k8s-context "$(K8S_CONTEXT)" \
	        --k8s-namespace "$(K8S_NAMESPACE)" --k8s-app "$(K8S_APP)"; \
	    fi ;; \
	  *) \
	    echo "!! 未知 env=$(ENV)（可用：local / k8s / dev / dev1 / dev2 / dev3 / test）"; \
	    exit 2 ;; \
	esac

build:
	go build -o $(BINARY) $(PKG)

# 内置 GORM 演示，无需真实服务即可体验
demo: build
	(cd examples/gorm-demo && CGO_ENABLED=1 go run .) | ./$(BINARY) --stdin

# 在当前目录生成 sql-monitor.json（不覆盖已有文件）
init: build
	./$(BINARY) init

test:
	CGO_ENABLED=1 go test -race ./...

test-demo:
	(cd examples/gorm-demo && CGO_ENABLED=1 go test -race ./...)

vet:
	go vet ./...
	(cd examples/gorm-demo && go vet ./...)

clean:
	rm -f $(BINARY)

help:
	@echo "常用命令（数据源由 env 决定，大小写均可）："
	@echo "  make                本地日志（默认，自动带配置开飞书）"
	@echo "  make run env=local  本地日志（显式）"
	@echo "  make run env=k8s    集群全部 data-service pod（忽略 lane）"
	@echo "  make run env=dev2   集群日志（指定 lane dev2）"
	@echo "  集群连接参数可覆盖：K8S_KUBECONFIG / K8S_CONTEXT / K8S_NAMESPACE / K8S_APP"
	@echo "  make demo            内置 GORM 演示，无需真实服务"
	@echo "  make init            在当前目录生成 sql-monitor.json"
	@echo "  make test / test-demo / vet   测试与静态检查"
	@echo "  make clean           删除编译产物"
