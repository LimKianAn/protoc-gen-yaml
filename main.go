package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"sort"

	"github.com/go-yaml/yaml"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

type T struct {
	Messages []Message `yaml:"messages"`
	Services []Service `yaml:"services"`
}

type Message struct {
	Name   string  `yaml:"name"`
	Fields []Field `yaml:"fields"`
}

type Field struct {
	Name   string `yaml:"name"`
	Number int32  `yaml:"number"`
}

type Service struct {
	Name    string   `yaml:"name"`
	Methods []Method `yaml:"methods"`
}

type Method struct {
	Name       string `yaml:"name"`
	InputType  string `yaml:"input_type"`
	OutputType string `yaml:"output_type"`
}

func main() {
	bb, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("reading stdin: %v", err)
	}

	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(bb, req); err != nil {
		log.Fatalf("parsing request: %v", err)
	}
	if len(req.FileToGenerate) == 0 {
		log.Fatal("no input file")
	}

	for _, file := range req.GetProtoFile() {
		pkg := file.GetPackage()

		pbMsgs := []*descriptorpb.DescriptorProto{}
		var f func(pbMsg *descriptorpb.DescriptorProto)
		f = func(pbMsg *descriptorpb.DescriptorProto) {
			parentName := pbMsg.GetName()
			if nested := pbMsg.GetNestedType(); len(nested) != 0 {
				for _, n := range nested {
					n.Name = proto.String(parentName + "." + n.GetName())
					f(n)
				}
			}
			pbMsgs = append(pbMsgs, pbMsg)
		}
		for _, pbMsg := range file.MessageType {
			f(pbMsg)
		}
		sort.Slice(pbMsgs, func(i, j int) bool {
			return pbMsgs[i].GetName() < pbMsgs[j].GetName()
		})

		msgs := []Message{}
		for _, pbMsg := range pbMsgs {
			pbMsg.GetNestedType()
			msg := Message{Name: pkg + "." + pbMsg.GetName()}
			sort.Slice(pbMsg.Field, func(i, j int) bool {
				return pbMsg.Field[i].GetName() < pbMsg.Field[j].GetName()
			})
			for _, field := range pbMsg.Field {
				msg.Fields = append(msg.Fields, Field{
					Name:   field.GetName(),
					Number: field.GetNumber(),
				})
			}
			msgs = append(msgs, msg)
		}
		sort.Slice(msgs, func(i, j int) bool {
			return msgs[i].Name < msgs[j].Name
		})

		svcs := []Service{}
		for _, protoSvc := range file.Service {
			svc := Service{Name: pkg + "." + protoSvc.GetName()}
			for _, method := range protoSvc.Method {
				svc.Methods = append(svc.Methods, Method{
					Name:       method.GetName(),
					InputType:  method.GetInputType()[1:],
					OutputType: method.GetOutputType()[1:],
				})
			}
			svcs = append(svcs, svc)
		}
		sort.Slice(svcs, func(i, j int) bool {
			return svcs[i].Name < svcs[j].Name
		})

		bb, err := yaml.Marshal(T{
			Messages: msgs,
			Services: svcs,
		})
		if err != nil {
			log.Fatalf("marshaling: %v", err)
		}

		resp := &pluginpb.CodeGeneratorResponse{}
		filename := *file.Name + ".yaml"
		resp.File = []*pluginpb.CodeGeneratorResponse_File{
			{
				Name:    proto.String(filename),
				Content: proto.String(string(bb)),
			},
		}

		bb, err = proto.Marshal(resp)
		if err != nil {
			log.Fatalf("marshaling code generator response: %v", err)
		}
		_, err = io.Copy(os.Stdout, bytes.NewReader(bb))
		if err != nil {
			log.Fatal("copying marshaled response to stdout")
		}
	}
}
