FROM golang:1.4

# Godep for vendoring
RUN go get github.com/tools/godep
ENV APP_DIR $GOPATH/src/github.com/wearemolecule/worker_cleaner
RUN mkdir /opt/app

# Set the entrypoint
ENTRYPOINT ["/opt/app/worker_cleaner"]
ADD . $APP_DIR

# Compile the binary and statically link
RUN cd $APP_DIR && godep restore
RUN cd $APP_DIR && go build -o /opt/app/worker_cleaner
