FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /spike .

FROM alpine:3.24
RUN apk add --no-cache iproute2 iptables iputils-arping
COPY --from=build /spike /usr/local/bin/spike
ENTRYPOINT ["/usr/local/bin/spike"]
