
all: build_bin

fetch:
	go get -u gopkg.in/ini.v1
	go get -u github.com/go-sql-driver/mysql
	touch $@

build_bin:
	go build -buildmode=exe -ldflags "-w -s" -o mysql-readonly-lock main.go
	touch $@

clean:
	rm -vf mysql-readonly-lock
	rm -vf fetch build_bin

.PHONY: clean fetch build_bin
