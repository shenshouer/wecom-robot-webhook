FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
COPY ./bin/wecom-robot-webhook /usr/bin
CMD ["wecom-robot-webhook"]