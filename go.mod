module loganalyzer

go 1.24.0

toolchain go1.24.11

replace github.com/sdcio/sdc-protos => /home/mava/projects/sdc-protos

require (
	github.com/sdcio/sdc-protos v0.0.47
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/sdcio/logger v0.0.3 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
	google.golang.org/grpc v1.78.0 // indirect
)
