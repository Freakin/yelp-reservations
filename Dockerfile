FROM golang:alpine AS build-env
WORKDIR /app
COPY . .
RUN apk add git
RUN go get ./... && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s" -o yelp-reservations

FROM scratch
WORKDIR /app
COPY --from=build-env /app/yelp-reservations /app/
CMD [ "/app/yelp-reservations" ]