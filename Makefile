
export GO111MODULE=on

all: build_bin

fetch:
	go get -u gopkg.in/ini.v1@v1.60.2
	go get -u github.com/go-sql-driver/mysql@v1.5.0
	touch $@

build_bin:
	go build -buildmode=exe -ldflags "-w -s" -o mysql-readonly-lock main.go
	touch $@

clean:
	rm -vf mysql-readonly-lock
	rm -vf fetch build_bin

.PHONY: clean fetch build_bin
