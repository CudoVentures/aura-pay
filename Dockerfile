FROM amd64/golang:1.18-buster AS builder
USER root

RUN apt-get update
WORKDIR /go/src/github.com/CudoVentures/aura-pay
COPY . ./
RUN go mod tidy -compat=1.18
RUN make build


FROM amd64/golang:1.18-buster
USER root

WORKDIR /cudos-markets-pay
COPY ./.env ./.env

COPY --from=builder /go/src/github.com/CudoVentures/aura-pay/build/cudos-markets-pay /usr/bin/cudos-markets-pay

CMD ["/bin/bash", "-c", "cudos-markets-pay"]
