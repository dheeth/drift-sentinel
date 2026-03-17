FROM golang:1.26 AS builder

WORKDIR /src

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY pkg ./pkg

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/drift-sentinel ./cmd/server

FROM gcr.io/distroless/static-debian12

COPY --from=builder /out/drift-sentinel /drift-sentinel

EXPOSE 8080

ENTRYPOINT ["/drift-sentinel"]
