FROM golang:1.23-alpine

ARG GOPROXY=https://proxy.golang.org|https://goproxy.cn|direct
ARG GOSUMDB=sum.golang.org

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

RUN apk add --no-cache python3 py3-pip sqlite && \
    python3 -m venv /opt/factorlab-venv && \
    /opt/factorlab-venv/bin/pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple \
      requests pandas numpy akshare baostock

ENV PATH="/opt/factorlab-venv/bin:${PATH}"

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
