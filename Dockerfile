FROM golang:1.10-alpine

RUN apk add make curl git

RUN curl https://glide.sh/get | sh

COPY . /go/src/github.com/wearemolecule/worker_cleaner
WORKDIR /go/src/github.com/wearemolecule/worker_cleaner
RUN make setup

RUN make build

ENTRYPOINT ["./worker_cleaner", "-logtostderr=true", "-v=0"]
