GO=CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go
BIN=worker_cleaner
IMAGE=quay.io/molecule/$(BIN)
DOCKER_TAG=latest
IMAGE_TAG=$(IMAGE):$(DOCKER_TAG)

all: image
	docker push $(IMAGE_TAG)

setup:
	glide install -v

build:
	$(GO) build -a -installsuffix cgo -o $(BIN) .

image: build
	docker build -t $(IMAGE_TAG) .

run: 
	go run main.go -logtostderr=true -v=10

.PHONY: clean

gtest:
	go test $$(glide nv)

clean:
	rm $(BIN)
