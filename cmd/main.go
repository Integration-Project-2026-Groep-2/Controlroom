package main


import (
	   "integration-project-ehb/controlroom/consumer"
)


func main() {

	// start the consumer and try to receive heartbeat messages
	consumer.Start() 
}
