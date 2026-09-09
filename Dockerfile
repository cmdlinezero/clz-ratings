FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /rating-api .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /rating-api /rating-api
ENV PORT=8080
ENV RATINGS_STORAGE_URI=gs://clz-certin/ratings
EXPOSE 8080
ENTRYPOINT ["/rating-api"]
