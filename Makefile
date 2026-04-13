help: Makefile
	@echo " Choose a command to run :"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'

## plugins: build all command plugins
plugins:
	go build -ldflags="-s -w" -buildmode=plugin -o commands/g0.so commands/g0/g0.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/g01.so commands/g01/g01.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/divide360.so commands/divide360/divide360.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/divide360EnableDisable.so commands/divide360EnableDisable/divide360EnableDisable.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/rpm.so commands/rpm/rpm.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/moveRotary.so commands/moveRotary/moveRotary.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/loopStart.so commands/loopStart/loopStart.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/loopEnd.so commands/loopEnd/loopEnd.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/invalidCommand.so commands/invalidCommand/invalidCommand.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/g68.so commands/g68/g68.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/g69.so commands/g69/g69.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/g90.so commands/g90/g90.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/g91.so commands/g91/g91.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/delay.so commands/delay/delay.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/m30.so commands/m30/m30.go
	go build -ldflags="-s -w" -buildmode=plugin -o commands/m99.so commands/m99/m99.go
	go build -ldflags="-s -w" -buildmode=plugin -o  commands/g17.so commands/g17/g17.go
	go build -ldflags="-s -w" -buildmode=plugin -o  commands/workoffset.so commands/workoffset/workoffset.go

## main: build main.go
main:
	go build -ldflags="-s -w -X 'main.BuildTime=$$(date)'" -o jamun main.go
	chmod 777 ./jamun

## c: build the ethercat c abstraction library	
c:
	gcc -o libethercatinterface.so -Wall -g -shared -fPIC ethercatinterface.c -I/opt/etherlab/include /opt/etherlab/lib/libethercat.a

## exec: execute the compiled main application
exec:
	./jamun

## run: call go run on main.go
run:
	go run main.go

## rb: run binary, build main.go and execute the file
rb: main exec

## all: build plugins and main application code
all: plugins main

## clean: clean all the executables, plugins and every thing in the release folder
clean:
	rm ./jamun
	rm ./commands/*.so
	rm -rf ./release

test:
	$(echo "test")
	$(echo "mode: \"rel\"" > ./configs/envconfig.yaml)

## package: build plugins and main program then package them for release, expect VERSION as param, eg. make package VERSION=1.0.33.0
package: all
    ifeq ($(VERSION),)
		$(error Version should not be empty, provide a version e.g. make package VERSION=1.0.0)
    endif

	$(eval fileName := jamun_v$(VERSION))
	
	mkdir -p "./release"
	$(eval dir_path := ./release/$(fileName))
	mkdir -p $(dir_path)
	mkdir "$(dir_path)/commands"
	mkdir "$(dir_path)/configs"
	mkdir "$(dir_path)/scripts"
	cp ./jamun "$(dir_path)"
	cp ./scripts/*.sh "$(dir_path)/scripts"
	cp ./ethercatinterface.h "$(dir_path)"
	cp ./libethercatinterface.so "$(dir_path)"
	cp ./configs/*.* "$(dir_path)/configs"
	chmod 777 $(dir_path)/configs/*.*
	chmod 777 $(dir_path)/scripts/*.*
# cp ./www_v2/*.* "$(dir_path)/www_v2"
	rm -rf "$(dir_path)/configs/envconfig.yaml"
	cp ./commands/*.so "$(dir_path)/commands"
	chmod 777 $(dir_path)/commands/*.*
	chmod 777 $(dir_path)/jamun
	tar -cvzf "./release/$(fileName).tar.gz" "$(dir_path)"
	du -sh "$(dir_path)"
	rm -rf "$(dir_path)"
	cp "./release/$(fileName).tar.gz" /home/pi/ftp/
	
## push: push the release to release server
push:
    ifeq ($(VERSION),)
		$(error Version should not be empty, provide a version e.g. make release VERSION=1.0.0)
    endif
	~/gosrc/src/release-cli/release-cli release -z /home/pi/gosrc/src/EtherCAT/release/jamun_v$(VERSION).tar.gz -p ebaf0856-606e-11eb-95b7-00155da1e406 -v \"$(VERSION)\" -c "v1"

testread:
	@read module; \
	@echo "$$module";

## compress: Compress the files. Should install upx (sudo apt-get install upx)	
compress:
	chmod +x ./commands/*.so
	upx -9 -k ./commands/*.so
	upx -9 -k ./jamun
	rm -rf ./commands/*.so~
	rm -rf ./*.~