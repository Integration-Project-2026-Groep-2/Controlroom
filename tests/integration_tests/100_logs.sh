#!/bin/sh
for i in {0..100}
do
  go test logger_test.go helper.go
  go test logger_send_test.go helper.go
  echo $i
done
