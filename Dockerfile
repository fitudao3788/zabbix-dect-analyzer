FROM golang:1.27-trixie AS build

WORKDIR /src
COPY go.mod .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/zabbix-dect-analyzer .

FROM debian:trixie-slim

WORKDIR /app

# tspacketchkのバージョン指定
ENV TSPACKETCHK_VERSION=Ver1.0.1

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    git make gcc \
    zabbix-sender

RUN git clone -b ${TSPACKETCHK_VERSION} https://github.com/kaikoma-soft/tspacketchk /tmp/tspacketchk && \
    cd /tmp/tspacketchk && \
    make && \
    make install && \
    rm -rf /tmp/tspacketchk

RUN apt-get remove -y git make gcc && \
    apt-get autoremove -y && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /out/zabbix-dect-analyzer /app/zabbix-dect-analyzer

ENTRYPOINT [ "/app/zabbix-dect-analyzer" ]
