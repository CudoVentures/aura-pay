
# CLEANUP
rm aura-pay.out
rm aura-pay.test

# COMPILE AURA PAY SERVICE
go test -c ./cmd/aura-pay -cover -covermode=count -coverpkg=./...

# EXECUTE AURA PAY SERVICE
./aura-pay.test -test.coverprofile aura-pay.out &

sleep 20

echo "STOP INTEGRATION TESTS"
curl http://127.0.0.1:19999

# GO111MODULE=off go get github.com/wadey/gocovmerge
gocovmerge *.out > merged.cov
go tool cover -func=merged.cov | grep -E '^total\:' | sed -E 's/\s+/ /g'