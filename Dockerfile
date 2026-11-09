# Image de l'API : binaire statique sur une base distroless, sans shell.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY db ./db
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/kodi ./cmd/kodi

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kodi /kodi
EXPOSE 8080
ENTRYPOINT ["/kodi"]
CMD ["api"]
