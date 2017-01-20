#!/bin/bash
set -ex

gometalinter --deadline=300s --disable-all\
	--enable=aligncheck\
	--enable=deadcode\
	--enable=errcheck\
	--enable=gas\
	--enable=goconst\
  --enable=gocyclo\
  --cyclo-over=15\
	--enable=gofmt\
	--enable=goimports\
	--enable=gosimple\
  --enable=gotype\
	--enable=ineffassign\
	--enable=interfacer\
	--enable=lll --line-length=160\
	--enable=misspell\
	--enable=staticcheck\
	--enable=structcheck\
	--enable=unconvert\
	--enable=unused\
	--enable=varcheck\
	--enable=vet\
	--enable=vetshadow\
	./...

# Disabled linters:
# - controller code has similar watchers
# --enable=dupl\
# - comments are not linted
#	--enable=golint --min-confidence=0 --exclude=.pb.go --exclude="should have a package comment"\
# - parsing code has high cyclomatic complexity
