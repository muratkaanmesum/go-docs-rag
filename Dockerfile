FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /web ./cmd/web

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /web /web
ENV ADDR=0.0.0.0:7070
EXPOSE 7070
ENTRYPOINT ["/web"]
