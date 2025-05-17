FROM golang:1.24.2

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
COPY main.go ./

RUN go mod download
RUN go build -o /app/main

CMD [ "/app/main" ]