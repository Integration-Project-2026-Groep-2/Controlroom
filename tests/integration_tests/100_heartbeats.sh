for i in {0..100}
do
  go test heartbeat_test.go helper.go
  echo $i
done