`dputils` is a simple utility suite for working with Deskpro instances.

```bash
$ dputils --help                                                                                                                                                                     (master+) 16:10:54
Deskpro tools and utilities for working with helpdesk instances

Usage:
  dputils [flags]
  dputils [command]

Available Commands:
  backup      Backup database and/or attachments to the archive
  dump_config Dumps current Deskpro config
  help        Help about any command
  restore     Restore a Deskpro instance to the current server.
  version     Print the version number

Flags:
      --deskpro string   Path to Deskpro on the current server
  -h, --help             help for dputils
      --php string       Path to PHP

Use "dputils [command] --help" for more information about a command.
```

# Official Builds

You can download the binary for your platform from the [Releases page](https://github.com/deskpro/dputils/releases).

Every copy of Deskpro also ships with the following binaries: linux/amd64 linux/386 windows/amd64 windows/386 darwin/amd64

You can run the tool from Deskpro using `bin/console` wrapper:

```bash
$ cd /path/to/deskpro
$ bin/console dputils --help
```

The wrapper selects the proper binary for your platform.

# Building

First, [download and install Go](https://golang.org/doc/install). Then you can build using the standard `go build` command:

```bash
$ go build
```

On Linux/Mac you can use the Makefile to build for all standard platforms:

```bash
$ make -j8
rm -rf build/
mkdir -p build
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o build/dputils_linux_amd64
GOOS=linux GOARCH=386 go build -ldflags "-s -w" -o build/dputils_linux_386
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o build/dputils_windows_amd64
GOOS=windows GOARCH=386 go build -ldflags "-s -w" -o build/dputils_windows_386
GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o build/dputils_darwin_amd64
zip -9 build/builds.zip build/dputils_*
  adding: build/dputils_darwin_amd64 (deflated 67%)
  adding: build/dputils_linux_386 (deflated 62%)
  adding: build/dputils_linux_amd64 (deflated 66%)
  adding: build/dputils_windows_386 (deflated 62%)
  adding: build/dputils_windows_amd64 (deflated 66%)
  
# build/builds.zip is what we ship in Deskpro distro
cp build/builds.zip /path/to/dev/deskpro/app/BUILD/bin/dputils_builds.zip
```
