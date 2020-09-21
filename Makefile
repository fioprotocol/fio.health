
local:
	go build -ldflags "-s -w" -o dist/fio-health fio-health/main.go

lambda:
	mkdir -p dist
	rm -f dist/deployment.zip dist/main
	GOOS=linux go build -ldflags "-s -w" -o dist/main fio-health/main.go
	cd dist && zip deployment.zip main GeoLite2-Country.mmdb

