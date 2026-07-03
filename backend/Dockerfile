ARG BACKEND_BASE_IMAGE=backend-runtime-base
FROM golang:1.23-alpine AS backend-runtime-base

ARG ALPINE_MIRROR=https://mirrors.tencent.com/alpine
ARG PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple

RUN sed -i "s#https://dl-cdn.alpinelinux.org/alpine#${ALPINE_MIRROR}#g" /etc/apk/repositories && \
    apk add --no-cache python3 py3-pip sqlite && \
    python3 -m venv /opt/factorlab-venv

COPY factorlab-requirements.txt /tmp/factorlab-requirements.txt
RUN /opt/factorlab-venv/bin/pip install --no-cache-dir -i "${PIP_INDEX_URL}" -r /tmp/factorlab-requirements.txt && \
    rm -f /tmp/factorlab-requirements.txt

ENV PATH="/opt/factorlab-venv/bin:${PATH}"

# Local docker compose builds default to the in-file base stage above so they do
# not depend on a pre-pushed registry base image or its platform manifest.
# Release scripts can still pass BACKEND_BASE_IMAGE=<registry>/pumpkin-base:<tag>
# to reuse the prebuilt backend runtime base.
FROM ${BACKEND_BASE_IMAGE}

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
