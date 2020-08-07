#!/bin/bash -e

cd tests
go test -v -count=1 -race -p 1
