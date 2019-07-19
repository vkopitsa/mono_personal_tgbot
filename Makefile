RELNAME=mono_personal_tgbot
GOARCH=amd64

# Setup the -ldflags option for go build here, interpolate the variable values
LDFLAGS = -ldflags "-s -w"

test: ## test
	go test ./...

build: linux darwin windows ## Build

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} go build ${LDFLAGS} -o ./release/${RELNAME}-linux-${GOARCH} *.go

darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=${GOARCH} go build ${LDFLAGS} -o ./release/${RELNAME}-darwin-${GOARCH} *.go

windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=${GOARCH} go build ${LDFLAGS} -o ./release/${RELNAME}-windows-${GOARCH}.exe *.go

up: ## To up all containers
	docker-compose up --build -d

down: ## To down all containers
	docker-compose stop

fmt: ## fmt
	go fmt ./...

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
