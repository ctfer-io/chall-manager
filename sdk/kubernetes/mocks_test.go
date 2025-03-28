package kubernetes_test

import (
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks struct{}

func (mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	outputs := args.Inputs.Mappable()
	// fmt.Printf("args.TypeToken: %v\n", args.TypeToken)
	// fmt.Printf("outputs: %v\n", outputs)
	switch args.TypeToken {
	case "kubernetes:core/v1:Service":
		// If Service is NodePort, give it a real one in the pool
		spec := outputs["spec"].(map[string]any)
		if spec["type"].(string) == "NodePort" {
			spec["ports"].([]any)[0].(map[string]any)["nodePort"] = 30001
		}
	}
	return args.Name + "_id", resource.NewPropertyMapFromMap(outputs), nil
}

func (mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}
