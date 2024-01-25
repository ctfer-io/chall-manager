package integration_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var Base string = ""

func TestMain(m *testing.M) {
	base, ok := os.LookupEnv("K8S_BASE")
	if !ok {
		out, err := exec.Command("minikube", "ip").Output()
		if err != nil {
			fmt.Println("Environment variable K8S_BASE is not set, please indicate the domain name/IP address to reach out the cluster.")
			os.Exit(1)
		}
		base = strings.TrimRight(string(out), "\n")
	}
	Base = base

	os.Exit(m.Run())
}
