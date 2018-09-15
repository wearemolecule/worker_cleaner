FROM alpine

RUN mkdir -p /opt/

RUN make build
COPY worker_cleaner /opt/worker_cleaner

ENTRYPOINT ["/opt/worker_cleaner", "-logtostderr=true", "-v=0"]
