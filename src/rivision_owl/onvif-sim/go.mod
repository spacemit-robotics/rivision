module github.com/gowvp/onvif-sim

go 1.25.0

require (
	github.com/0x524a/onvif-go v1.0.0
	github.com/sirupsen/logrus v1.9.3
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/stretchr/testify v1.10.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
)

replace github.com/0x524a/onvif-go => ../onvif-go
