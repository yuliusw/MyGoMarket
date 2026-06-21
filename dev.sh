go build -o ./script/main .
cd ./script
docker build -t rpa-app:v1.0 .
docker compose up -d
docker compose restart app
docker logs RPA-app
