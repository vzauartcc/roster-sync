FROM golang:1.26.6-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -v -o roster-sync ./cmd/roster-sync/main.go

FROM gcr.io/distroless/static-debian13

COPY --from=builder /app/roster-sync /

ENTRYPOINT ["./roster-sync"]
