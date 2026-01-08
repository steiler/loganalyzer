package main

import (
	"encoding/base64"
	"fmt"
	"log"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func main() {
	// Example from log: Eg4xOTIuMTY4LjEuMS8yNA==
	encoded := "Eg4xOTIuMTY4LjEuMS8yNA=="
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		log.Fatal(err)
	}

	tv := &sdcpb.TypedValue{}
	if err := proto.Unmarshal(data, tv); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Decoded:", tv)

	marshaller := protojson.MarshalOptions{Multiline: true}
	jsonStr, err := marshaller.Marshal(tv)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("JSON:", string(jsonStr))
}
