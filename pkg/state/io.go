package state

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func New(ctx context.Context, stack auto.Stack, metadata StateMetadata, outputs auto.OutputMap) (*State, error) {
	state := &State{
		Metadata: metadata,
	}

	// Export Pulumi state
	udp, err := stack.Export(ctx)
	if err != nil {
		return nil, err
	}
	state.Pulumi = udp.Deployment

	// Fetch outputs
	coninfo, ok := outputs["connection_info"]
	if !ok {
		return nil, &ErrOutputRequired{
			Key: "connection_info",
		}
	}
	state.Outputs.ConnectionInfo = coninfo.Value.(string)
	flag, ok := outputs["flag"]
	if ok {
		fstr := flag.Value.(string)
		state.Outputs.Flag = &fstr
	}

	return state, nil
}

func (state *State) Export(identity string) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(global.Conf.Directory, "states", identity), b, 0644)
}

func Load(identity string) (*State, error) {
	b, err := os.ReadFile(filepath.Join(global.Conf.Directory, "states", identity))
	if err != nil {
		return nil, err
	}
	state := &State{}
	if err := json.Unmarshal(b, state); err != nil {
		return nil, err
	}
	return state, nil
}
