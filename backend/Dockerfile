FROM golang:1.21-alpine

ARG GOPROXY=https://proxy.golang.org|https://goproxy.cn|direct
ARG GOSUMDB=sum.golang.org

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

WORKDIR /app

COPY go.mod go.sum ./
RUN echo "Using GOPROXY=${GOPROXY}" && echo "Using GOSUMDB=${GOSUMDB}" && \
    for i in 1 2 3; do \
      go mod download -x && exit 0; \
      echo "go mod download failed (attempt ${i}/3), retrying..."; \
      sleep $((i * 5)); \
    done; \
    exit 1

COPY . .

RUN go build -trimpath -o main .
CMD ["/app/main"]
