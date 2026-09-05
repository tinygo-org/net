#!/bin/sh
set -eu

tinygo_bin=${TINYGO_BIN:-tinygo}
release_root=$("$tinygo_bin" env TINYGOROOT)
net_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/net-listenconfig.XXXXXX")

tar -C "$release_root" --exclude='./src/net' -cf - ./src | tar -C "$test_root" -xf -
mkdir "$test_root/src/net"
tar -C "$net_root" --exclude='./.git' -cf - . | tar -C "$test_root/src/net" -xf -
ln -s "$release_root/lib" "$test_root/lib"
ln -s "$release_root/targets" "$test_root/targets"

TINYGOROOT="$test_root" "$tinygo_bin" test -c -o "$test_root/listenconfig.test" net
printf 'Test binary: %s\n' "$test_root/listenconfig.test"
printf 'Run on the target OS with an external timeout and -test.run TestListenConfig\n'
