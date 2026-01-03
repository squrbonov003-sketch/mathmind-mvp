FROM golang:1.21 as builder
WORKDIR /app
COPY go.mod ./
COPY migrations ./migrations
COPY cmd ./cmd
RUN go mod download || true
RUN CGO_ENABLED=0 GOOS=linux go build -o mathmind ./cmd/server

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /app/mathmind ./mathmind
ENV PORT=8080
EXPOSE 8080
CMD ["./mathmind"]
