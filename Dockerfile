FROM golang:1.21-bookworm AS build
WORKDIR /app
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o server

FROM debian:bookworm-slim
RUN useradd -m app
WORKDIR /home/app
COPY --from=build /app/server .
ENV PORT=8080
EXPOSE 8080
USER app
CMD ["./server"]
