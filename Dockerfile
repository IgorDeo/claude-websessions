FROM golang:1.22-alpine AS builder

RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN templ generate ./web/templates/
RUN CGO_ENABLED=0 go build -o websessions ./cmd/websessions

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/websessions /usr/local/bin/websessions

EXPOSE 8080
ENTRYPOINT ["websessions"]
