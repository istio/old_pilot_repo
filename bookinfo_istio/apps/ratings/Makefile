APP_NAME 	:= $(shell basename `pwd`)
IMAGE_NAME      := $(APP_NAME)

BUILDFLAGS  :=
LDFLAGS 	:= -linkmode external -extldflags -static

# Integration test runs within docker containers only
GOOS        := linux
GOARCH      := amd64

build:
	@go build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' -o $(APP_NAME)
	
build.dockerize:
	@docker build -t $(IMAGE_NAME)-v1:latest .

docker.push:
	@docker tag $(IMAGE_NAME)-v1:latest $(HUB)/$(IMAGE_NAME)-v1:latest
	@docker push $(HUB)/$(IMAGE_NAME)-v1:latest

clean:
	@rm $(APP_NAME)
