package main


import (
	   "controlroom/consumer"
)


func main() {

	// start the consumer and try to receive heartbeat messages
	consumer.Start() 
}
