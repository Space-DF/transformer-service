FROM alpine:3.18
RUN apk add --no-cache logrotate
CMD ["sh", "-c", "/usr/sbin/logrotate /etc/logrotate.d/transformer && while true; do sleep 86400 & wait $!; /usr/sbin/logrotate /etc/logrotate.d/transformer; done"]
