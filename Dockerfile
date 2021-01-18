FROM brycehuang/debian10:golang-1.15.6 as builder
#FROM golang:alpine as builder
RUN mkdir -p /go/src
COPY *.go /go/src
COPY go.mod /go/src
RUN go env -w GO111MODULE=on && go env -w GOPROXY=https://goproxy.cn,direct
RUN cd /go/src && go build -o init  main.go

#FROM alpine
FROM bitnami/mariadb-galera:10.5.8-debian-10-r0
COPY --from=builder /go/src/init /init
#COPY hello.sh /hello.sh
COPY init.sh /init.sh
WORKDIR /

#ENTRYPOINT ["./init","&&","/opt/bitnami/scripts/mariadb-galera/entrypoint.sh","/opt/bitnami/scripts/mariadb-galera/run.sh"]
ENTRYPOINT ["./init.sh"]