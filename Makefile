PLATFORMS := linux/amd64 linux/386 windows/amd64 windows/386 darwin/amd64
PACK_EXECUTABLES := 1

temp = $(subst /, ,$@)
os   = $(word 1, $(temp))
arch = $(word 2, $(temp))
sfx  = $(if $(findstring windows,$(os)),.exe,)

all: clean
	$(MAKE) build

build: build_bins build_zip

$(PLATFORMS): install_deps
	@if [ ! -d build ]; then mkdir -p build; fi
	GOOS=$(os) GOARCH=$(arch) go build -ldflags "-s -w" -o build/dputils_$(os)_$(arch)$(sfx)
	test "$(PACK_EXECUTABLES)" = 1 && upx --best build/dputils_$(os)_$(arch)$(sfx)

build_bins: $(PLATFORMS)

build_zip: build_bins
	cd build; zip -9 builds.zip dputils_*

clean:
	rm -rf build/
	mkdir -p build

install_deps:
	GOOS=linux go get -d -t -v ./...
	GOOS=windows go get -d -t -v ./...
	GOOS=darwin go get -d -t -v ./...

.PHONY: build build_bins build_zip clean $(PLATFORMS)
