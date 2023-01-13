FROM amd64/golang:1.18-buster AS builder
USER root

RUN apt-get update
WORKDIR /go/src/github.com/CudoVentures/aura-pay
COPY . ./
RUN go mod tidy -compat=1.18
RUN make build


FROM amd64/golang:1.18-buster
USER root

WORKDIR /aura-pay
COPY ./.env ./.env

COPY --from=builder /go/src/github.com/CudoVentures/aura-pay/build/aura-pay /usr/bin/aura-pay

CMD ["/bin/bash", "-c", "aura-pay"]
