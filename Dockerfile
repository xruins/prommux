FROM golang AS builder

COPY . ./
LABEL org.opencontainers.image.source="https://github.com/xruins/prommux"

RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /app/prommux

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=builder --chmod=755 /app/prommux /usr/local/bin/prommux
ENTRYPOINT ["/usr/local/bin/prommux"]
EXPOSE 11298