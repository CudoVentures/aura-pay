
go test -timeout 30s -v -cover -covermode=count -coverprofile unittests.out ./internal/...

retVal=$?
if [ $retVal -ne 0 ]; then
    exit $retVal
fi

go tool cover -func=unittests.out | grep -E '^total\:' | sed -E 's/\s+/ /g'

COVERAGE=$(go tool cover -func unittests.out | grep total | awk '{print substr($3, 1, length($3)-1)}')

echo "Tests coverage $COVERAGE"
