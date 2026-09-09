FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
# If you have a go.sum file, uncomment the line below to prevent module verification errors:
# COPY go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /certin-api .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /certin-api /certin-api
ENV PORT=8080
ENV RATINGS_STORAGE_URI=gs://clz-certin/ratings
EXPOSE 8080
ENTRYPOINT ["/certin-api"]
