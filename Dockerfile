FROM golang AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /app/prommux

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=builder --chmod=755 /app/prommux /usr/local/bin/prommux
ENTRYPOINT ["/usr/local/bin/prommux", "server"]
HEALTHCHECK CMD ["/usr/local/bin/prommux", "healthcheck"]
EXPOSE 11298
LABEL org.opencontainers.image.source="https://github.com/xruins/prommux"
