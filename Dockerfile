FROM alpine

RUN mkdir -p /opt/

COPY worker_cleaner /opt/worker_cleaner

ENTRYPOINT ["/opt/worker_cleaner"]
