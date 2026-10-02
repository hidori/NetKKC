.PHONY: test
test:
	@echo "Running tests..."
	@go test ./...

.PHONY: build
build:
	@echo "Building the project..."
	@go build ./main.go -o bin/kkc-server

.PHONY: run
run:
	@echo "Running the project..."
	@go run ./main.go -o bin/kkc-server

.PHONY: clean
clean:
	@echo "Cleaning the project..."
	@go clean .
	@go clean -cache -testcache
