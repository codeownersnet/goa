FROM golang:1.27 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -o /goafl ./cmd/goafl

FROM gcr.io/distroless/base-debian12

COPY --from=builder /goafl /goafl

ENTRYPOINT ["/goafl"]
