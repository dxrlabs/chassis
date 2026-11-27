module github.com/dxrlabs/chassis/broker/kafka

go 1.26.0

require (
	github.com/dxrlabs/chassis/broker v0.0.0
	github.com/segmentio/kafka-go v0.4.51
)

require (
	github.com/klauspost/compress v1.20.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.33 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

replace github.com/dxrlabs/chassis/broker => ../
