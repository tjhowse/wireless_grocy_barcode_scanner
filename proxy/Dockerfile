FROM golang:1.25-alpine AS build

RUN apk update && apk upgrade

WORKDIR /wireless_grocy_barcode_scanner_proxy

COPY *.go go.sum go.mod /wireless_grocy_barcode_scanner_proxy/

RUN go build -o /wireless_grocy_barcode_scanner_proxy/wireless_grocy_barcode_scanner_proxy

FROM alpine:latest AS runtime

RUN apk add --no-cache ca-certificates && update-ca-certificates

WORKDIR /wireless_grocy_barcode_scanner_proxy

COPY --from=build /wireless_grocy_barcode_scanner_proxy/wireless_grocy_barcode_scanner_proxy /wireless_grocy_barcode_scanner_proxy/wireless_grocy_barcode_scanner_proxy

CMD ["/wireless_grocy_barcode_scanner_proxy/wireless_grocy_barcode_scanner_proxy"]