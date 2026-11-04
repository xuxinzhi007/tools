.PHONY: toolbox toolbox-stop bootstrap

BIN ?= $(HOME)/bin/toolbox

toolbox:
	cd toolbox && go build -o $(BIN) .
	$(BIN)

toolbox-stop:
	$(BIN) -stop

bootstrap:
	@echo "换机器恢复：先在工具箱首页下载 toolbox-bootstrap.sh"
	@echo "手动补 ~/.toolbox/config.yaml 后执行 make toolbox"
