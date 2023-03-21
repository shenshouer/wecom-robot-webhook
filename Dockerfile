FROM alpine:latest
COPY ./bin/wecom-robot-webhook /usr/bin
ENTRYPOINT ["/wecom-robot-webhook"]