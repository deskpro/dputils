PLATFORMS := linux/amd64 linux/386 windows/amd64 windows/386 darwin/amd64

temp = $(subst /, ,$@)
os   = $(word 1, $(temp))
arch = $(word 2, $(temp))
sfx  = $(if $(findstring windows,$(os)),.exe,)

all: clean
	$(MAKE) build

build: build_bins build_zip

$(PLATFORMS):
	@if [ ! -d build ]; then mkdir -p build; fi
	GOOS=$(os) GOARCH=$(arch) go build -ldflags "-s -w" -o build/dputils_$(os)_$(arch)$(sfx)

build_bins: $(PLATFORMS)

build_zip: build_bins
	zip -9 build/builds.zip build/dputils_*

clean:
	rm -rf build/
	mkdir -p build

.PHONY: build build_bins build_zip clean $(PLATFORMS)
