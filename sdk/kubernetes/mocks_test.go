package kubernetes_test

import (
	"math/rand/v2"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks struct{}

func (mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	outputs := args.Inputs.Mappable()
	switch args.TypeToken {
	case "kubernetes:core/v1:Service":
		// If Service is NodePort, give it a real one in the pool
		spec := outputs["spec"].(map[string]any)
		switch spec["type"].(string) {
		case "NodePort":
			spec["ports"].([]any)[0].(map[string]any)["nodePort"] = 30000 + rand.Int()%2768 // kubernetes base range

		case "LoadBalancer":
			spec["ports"].([]any)[0].(map[string]any)["nodePort"] = 30000 + rand.Int()%2768 // kubernetes base range
			spec["externalIPs"] = []string{"some-random.host.tld"}                          // simulate some random external IP assigned to the service
		}
	}
	return args.Name + "_id", resource.NewPropertyMapFromMap(outputs), nil
}

func (mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}
