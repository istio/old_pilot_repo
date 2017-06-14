APP_NAME 	:= $(shell basename `pwd`)

BUILDFLAGS  :=
LDFLAGS 	:= -linkmode external -extldflags -static

# Integration test runs within docker containers only
GOOS        := linux
GOARCH      := amd64

build:
	@go build $(BUILDFLAGS) -ldflags '$(LDFLAGS)' -o $(APP_NAME)
	
clean:
	@rm $(APP_NAME)
