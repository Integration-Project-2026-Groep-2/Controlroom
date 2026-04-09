#!/bin/sh
# NOTE(nasr): while looop for testing LOTS of companys
while true; do go test producer_generated_test.go -count=1 .; sleep 1; done
