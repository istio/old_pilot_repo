APP_NAME 	:= $(shell basename `pwd`)
IMAGE_NAME      := $(APP_NAME)

BUILDFLAGS  :=
LDFLAGS 	:= -linkmode external -extldflags -static

# Integration test runs within docker containers only
GOOS        := linux
GOARCH      := amd64

build:
	@go build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' -o $(APP_NAME)

build.dockerize: build.dockerize-v1 build.dockerize-v2 build.dockerize-v3

build.dockerize-v1:
	@docker build -t $(IMAGE_NAME)-v1:latest -f Dockerfile.v1 .

build.dockerize-v2:
	@docker build -t $(IMAGE_NAME)-v2:latest -f Dockerfile.v2 --build-arg enable_ratings=true .

build.dockerize-v3:
	@docker build -t $(IMAGE_NAME)-v3:latest -f Dockerfile.v3 --build-arg enable_ratings=true --build-arg star_color=red .

docker.push: docker.push-v1 docker.push-v2 docker.push-v3

docker.push-v1:
	@docker tag $(IMAGE_NAME)-v1:latest $(HUB)/$(IMAGE_NAME)-v1:latest
	@docker push $(HUB)/$(IMAGE_NAME)-v1:latest

docker.push-v2:
	@docker tag $(IMAGE_NAME)-v2:latest $(HUB)/$(IMAGE_NAME)-v2:latest
	@docker push $(HUB)/$(IMAGE_NAME)-v2:latest

docker.push-v3:
	@docker tag $(IMAGE_NAME)-v3:latest $(HUB)/$(IMAGE_NAME)-v3:latest
	@docker push $(HUB)/$(IMAGE_NAME)-v3:latest

clean:
	@rm $(APP_NAME)
