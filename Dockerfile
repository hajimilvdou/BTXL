FROM node:22-alpine AS panel-builder

WORKDIR /panel

COPY internal/panel/web/package.json ./package.json
COPY internal/panel/web/tsconfig.json ./tsconfig.json
COPY internal/panel/web/vite.config.ts ./vite.config.ts
COPY internal/panel/web/index.html ./index.html
COPY internal/panel/web/src ./src

RUN npm install --no-fund --no-audit
RUN npm run build

FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

COPY --from=panel-builder /panel/dist ./internal/panel/web/dist

RUN apk add --no-cache git && go mod tidy -v 2>&1

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./btxl ./cmd/server/

FROM alpine:3.22.0

RUN apk add --no-cache tzdata

RUN mkdir -p /opt/btxl /opt/btxl/config /opt/btxl/logs /root/.btxl

COPY --from=builder ./app/btxl /opt/btxl/btxl

COPY config.example.yaml /opt/btxl/config.example.yaml

COPY docker/entrypoint.sh /usr/local/bin/btxl-entrypoint.sh

RUN chmod +x /usr/local/bin/btxl-entrypoint.sh

WORKDIR /opt/btxl

EXPOSE 8317

ENV TZ=Asia/Shanghai

RUN cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

ENTRYPOINT ["/usr/local/bin/btxl-entrypoint.sh"]
