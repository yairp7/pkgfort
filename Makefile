BINARY  := build/pkgfort
CMD     := ./cmd/pkgfort

.PHONY: build install clean test-docker

build:
	mkdir -p build
	go build -o $(BINARY) $(CMD)

install: build
	./$(BINARY) install

unit-tests:
	go test ./...

test-docker:
	docker build --no-cache -f Dockerfile .

clean:
	rm -rf build
