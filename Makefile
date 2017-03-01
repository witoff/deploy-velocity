
run-local:
	go run src/*  -c config/config.dev.yaml  -v

deploy: clean build-lambda
	serverless deploy

build: clean
	  go build -o main

build-lambda: clean
	GOOS=linux GOARCH=amd64 go build -o main ./src

test: main
	go test `find . | grep '_test\.go$$' | sort | xargs -n 1 dirname`

clean:
	rm -f main
