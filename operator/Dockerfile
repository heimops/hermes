FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o manager .

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /app/manager /manager

ENTRYPOINT ["/manager"]
