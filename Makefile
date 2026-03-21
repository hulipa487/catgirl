.PHONY: build test clean run install-deps migrate

NAME := catgirl
VERSION := 0.1.0
BUILD_DIR := ./build
CMD_PATH := ./cmd/server

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(NAME) $(CMD_PATH)

build-cli:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(NAME)-cli ./cmd/cli

clean:
	rm -rf $(BUILD_DIR)

test:
	go test -v ./...

run: build
	$(BUILD_DIR)/$(NAME) server -c catgirl.conf.example

install-deps:
	go mod download
	go mod tidy

lint:
	golangci-lint run

fmt:
	go fmt ./...

vet:
	go vet ./...

migrate:
	@echo "Run migrations with: catgirl migrate -c /path/to/config"

docker-build:
	docker build -t $(NAME):$(VERSION) .

docker-run:
	docker run -p 8080:8080 $(NAME):$(VERSION)

all: clean install-deps build
