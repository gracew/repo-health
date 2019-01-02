FROM golang:1.11 as builder
RUN mkdir /build 
ADD . /build/
WORKDIR /build 
RUN GOOS=linux GOARCH=386 go build -o main .

FROM alpine
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/main /app/
WORKDIR /app
CMD ["./main"]