BINARY    := opswise
APP_DIR   := app
BUILD_DIR := $(APP_DIR)
GO        := PATH="$$PATH:/usr/local/go/bin:$$HOME/go/bin" go
TEMPL     := PATH="$$PATH:/usr/local/go/bin:$$HOME/go/bin" templ
SQLC      := PATH="$$PATH:/usr/local/go/bin:$$HOME/go/bin" sqlc

.PHONY: build run dev generate clean docker docker-run templ-generate sqlc-generate

## build: generate templates and compile Go binary
build: templ-generate
	cd $(BUILD_DIR) && $(GO) build -o ../$(BINARY) ./cmd/

## run: build then start the server
run: build
	./$(BINARY)

## dev: build and run with visible output
dev: build
	./$(BINARY)

## generate: regenerate templ + sqlc
generate: templ-generate sqlc-generate

## templ-generate: run templ generate
templ-generate:
	$(TEMPL) generate -f $(APP_DIR)/web/templates/

## sqlc-generate: run sqlc generate
sqlc-generate:
	cd $(APP_DIR) && $(SQLC) generate

## docker: build Docker image
docker:
	docker build -t opswise .

## docker-run: run Docker container
docker-run:
	docker run -d --name opswise -p 8080:8080 opswise

## clean: remove binary and database
clean:
	rm -f $(BINARY) opswise.db
