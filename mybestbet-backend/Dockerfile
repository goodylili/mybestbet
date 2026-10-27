FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 sh -c 'for b in ingestor browserd resolver engine notifier api stats seed migrate; do go build -trimpath -ldflags="-s -w" -o /out/$b ./cmd/$b; done'

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata chromium
WORKDIR /app
COPY --from=build /out /app/bin
COPY config.yaml /app/config.yaml
ENV ROD_BROWSER_BIN=/usr/bin/chromium-browser
