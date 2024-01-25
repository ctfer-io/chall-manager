package launch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/global"
)

func (server *launcherServer) RetrieveLaunch(ctx context.Context, req *RetrieveLaunchRequest) (*LaunchResponse, error) {
	id := identity(req.ChallengeId, req.SourceId)

	out, err := loadStateOutputs(ctx, id)
	if err != nil {
		return nil, err
	}

	return &LaunchResponse{
		ConnectionInfo: out["connection_info"],
	}, nil
}

func loadStateOutputs(ctx context.Context, id string) (map[string]string, error) {
	bs, err := os.ReadFile(filepath.Join(global.Conf.StatesDir, id))
	if err != nil {
		return nil, err
	}

	type state struct {
		Resources []struct {
			Type    string            `json:"type"`
			Outputs map[string]string `json:"outputs"`
		} `json:"resources"`
	}
	var st state
	if err := json.Unmarshal(bs, &st); err != nil {
		return nil, err
	}
	for _, res := range st.Resources {
		if res.Type == "pulumi:pulumi:Stack" {
			return res.Outputs, nil
		}
	}
	return nil, errors.New("pulumi stack not found in existing state")
}
