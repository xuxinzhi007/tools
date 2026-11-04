BIN ?= $(HOME)/bin/toolbox

.PHONY: build run install stop clean
build:
	go build -o $(BIN) .

run: build
	$(BIN)

stop:
	$(BIN) -stop

clean:
	rm -f $(BIN)
