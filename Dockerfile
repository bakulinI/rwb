FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o /app/trends ./cmd/trends

FROM alpine:3.22

COPY --from=build /app/trends /app/trends
EXPOSE 8080
ENTRYPOINT ["/app/trends"]
