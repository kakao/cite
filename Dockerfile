FROM golang:1.6

MAINTAINER Seung Lee <code0x9@gmail.com>

ENV TZ Asia/Seoul

ADD . /go/src/github.com/kakao/cite

WORKDIR /go/src/github.com/kakao/cite

RUN go-wrapper download
RUN go-wrapper install

EXPOSE 8080

CMD ["/go/bin/cite"]
