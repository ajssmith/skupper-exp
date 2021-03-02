VERSION := $(shell git describe --tags --dirty=-modified --always)
PUBLIC_IMAGE := quay.io/ajssmith/skupper-exp-controller
LOCAL_IMAGE := localhost:5000/skupper-exp-controller

all: build-cmd build-controller build-plugins

build-cmd:
	go build -ldflags="-X main.version=${VERSION}"  -o skupper-exp cmd/skupper-exp/main.go

build-controller:
	go build -ldflags="-X main.version=${VERSION}"  -o controller cmd/service-controller/main.go cmd/service-controller/controller.go cmd/service-controller/service_sync.go cmd/service-controller/bridges.go

build-plugins: build-docker-plugin build-podman-plugin

build-docker-plugin:
	go build -ldflags="-X main.version=${VERSION}" --buildmode=plugin -o docker.so plug-ins/docker/docker.go

build-podman-plugin:
	go build -ldflags="-X main.version=${VERSION}" --buildmode=plugin -o podman.so plug-ins/podman/podman.go

local-build:
	docker build -t ${LOCAL_IMAGE} -f Dockerfile .

local-push:
	docker push ${LOCAL_IMAGE}

public-build:
	docker build -t ${PUBLIC_IMAGE} -f Dockerfile .

public-push:
	docker push ${PUBLIC_IMAGE}

format:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf skupper-exp controller release

package: release/windows.zip release/darwin.zip release/linux.tgz

release/linux.tgz: release/linux/skupper-exp
	tar -czf release/linux.tgz -C release/linux/ skupper-exp

release/linux/skupper-exp: cmd/skupper-exp/main.go
	GOOS=linux GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/linux/skupper-exp cmd/skupper-exp/main.go

release/windows/skupper-exp: cmd/skupper-exp/main.go
	GOOS=windows GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/windows/skupper-exp.exe cmd/skupper-exp/main.go

release/windows.zip: release/windows/skupper-exp
	zip -j release/windows.zip release/windows/skupper-exp.exe

release/darwin/skupper-exp: cmd/skupper-exp/main.go
	GOOS=darwin GOARCH=amd64 go build -ldflags="-X main.version=${VERSION}" -o release/darwin/skupper-exp cmd/skupper-exp/main.go

release/darwin.zip: release/darwin/skupper-exp
	zip -j release/darwin.zip release/darwin/skupper-exp
