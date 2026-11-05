# Blockworld - shortcuts. The work is done by cx.sh (macOS, Linux, Git Bash on
# Windows) so there is one build recipe; build.ps1 is the PowerShell equivalent.
# raylib is compiled through cgo, so each package is built on its own platform,
# except Windows, which can also be cross-compiled with mingw-w64.

.PHONY: run build test vet check package universal dmg windows install deps icon clean \
        app-macos app-linux app-windows

# build and run from source
run:
	go run .

# compile ./blockworld (blockworld.exe on Windows)
build:
	./cx.sh --build-only

test:
	go test ./...

vet:
	go vet ./...

# what CI checks: formatting, vet and tests
check: vet test
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }

# test, build and package for this machine into dist/
package:
	./cx.sh

# macOS: one app for Apple silicon and Intel, plus a disk image
universal:
	./cx.sh --universal --dmg

# macOS: package with a disk image
dmg:
	./cx.sh --dmg

# cross-compile the Windows package from macOS or Linux (needs mingw-w64)
windows:
	./cx.sh --target windows

# package and install for the current user
install:
	./cx.sh --install

# install the build dependencies (Linux package manager, macOS tools), then build
deps:
	./cx.sh --deps --build-only

# regenerate icon.png
icon:
	go run ./cmd/mkicon

clean:
	./cx.sh --clean

# Older target names, kept working.
app-macos app-linux app-windows: package
