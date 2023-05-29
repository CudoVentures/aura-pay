# initialise postgres test db
docker stop cudos-markets-pay-test-postgres || true
docker rm cudos-markets-pay-test-postgres || true
docker run --name cudos-markets-pay-test-postgres  -e POSTGRES_USER=postgresUser -e POSTGRES_PASSWORD=mysecretpassword  -e POSTGRES_DB=cudos-markets-pay-test-db -d -p 5432:5432 postgres:14
# clone the repo and scaffold the DB
git clone -b dev https://github.com/CudoVentures/cudos-aura-pool-platform.git cudos-markets-pay-platform
cd ./cudos-markets-pay-platform || return 1
# Create env file
cd  ./config || return 1
cat <<EOF > .env
# app
App_Port='3001'

# Database
App_Database_Host='host.docker.internal'
App_Database_User='postgresUser'
App_Database_Pass='mysecretpassword'
App_Database_Db_Name='cudos-markets-pay-test-db'
App_Hasura_Url='http://127.0.01:8080/v1/graphql'

# LOCAL
APP_LOCAL_RPC='http://127.0.01:26657'
APP_LOCAL_API='http://127.0.01:1317'
APP_LOCAL_EXPLORER_URL='http://127.0.01:3000/'
APP_LOCAL_STAKING_URL='http://127.0.01:3000/staking'
GRAPHQL_URL="http://127.0.01/v1/graphql"
APP_LOCAL_CHAIN_NAME='CudosTestnet-test'
APP_LOCAL_CHAIN_ID='cudos-test-network'

# GENERAL SETTINGS
APP_DEFAULT_NETWORK='LOCAL'
APP_GAS_PRICE='5000000000000'
EOF

cd ../docker || return 1
# Set up environment file
cat <<EOF > .env
HOST_PORT="3001"
DOCKER_PORT="3001"

POSTGRES_PASSWORD="mysecretpassword"
POSTGRES_HOST_AUTH_METHOD=""
POSTGRES_DB=cudos-markets-pay-test-db
EOF

# Remove the old container if there is one
docker stop cudos-markets-platform-prod || true
docker rm cudos-markets-platform-prod || true

# Install the app
docker-compose --env-file ./prod.arg -f ./prod.yml -p cudos-markets-platform-prod up --build -d
rm -rf ../../cudos-markets-pay-platform

EXECUTE_DB_TEST=true

go test -timeout 30s -v -cover -covermode=count -coverprofile unittests.out ./internal/...

go tool cover -func=unittests.out | grep -E '^total\:' | sed -E 's/\s+/ /g'

COVERAGE=$(go tool cover -func unittests.out | grep total | awk '{print substr($3, 1, length($3)-1)}')

echo "Tests coverage $COVERAGE"

