#!/bin/sh
# Fetches the pinned corpus into ./corpus. Re-running is safe.
set -eu

WEBSITE_SHA=52d470bcf26e41fd5a38199c5463268fb61b67ae
GO_SHA=0d37007fda071631fbcb520b99c85a8e4c93108a

cd "$(dirname "$0")"
mkdir -p corpus/go/doc

if [ ! -d corpus/website/.git ]; then
	git init -q corpus/website
	git -C corpus/website remote add origin https://github.com/golang/website
fi
git -C corpus/website sparse-checkout set --no-cone _content/doc _content/ref _content/blog
git -C corpus/website fetch -q --depth 1 origin "$WEBSITE_SHA"
git -C corpus/website checkout -q FETCH_HEAD

for f in go_spec.html go_mem.html; do
	curl -fsSL "https://raw.githubusercontent.com/golang/go/$GO_SHA/doc/$f" -o "corpus/go/doc/$f"
done

echo "corpus ready: website@${WEBSITE_SHA%"${WEBSITE_SHA#???????}"} go@${GO_SHA%"${GO_SHA#???????}"}"
