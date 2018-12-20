
all: build

fetch:
	go get -u gopkg.in/ini.v1
	go get -u github.com/go-sql-driver/mysql
	touch $@

build: fetch
	go build -buildmode=exe -o mysql-readonly-lock main.go
	touch $@

clean:
	rm -vf mysql-readonly-lock
	rm -vf fetch build

.PHONY: clean fetch build
