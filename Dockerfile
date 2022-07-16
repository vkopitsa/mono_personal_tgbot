# builder
FROM golang:1.18-alpine as builder

WORKDIR /

RUN apk --update upgrade && \
   apk add git && \
   rm -rf /var/cache/apk/*

COPY . .

ENV GO111MODULE=on
RUN CGO_ENABLED=0 GOOS=linux go build -o /app .

# runner
FROM alpine:latest  
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /
COPY --from=builder /app .

# run it!
CMD ["./app"]