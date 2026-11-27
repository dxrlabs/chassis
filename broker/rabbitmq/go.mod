module github.com/dxrlabs/chassis/broker/rabbitmq

go 1.22

require (
	github.com/dxrlabs/chassis/broker v0.0.0
	github.com/rabbitmq/amqp091-go v1.17.0
)

replace github.com/dxrlabs/chassis/broker => ../
